// update.rs：自更新——GitHub API 查最新版、下载产物、ed25519 验签、解包、原子替换、sudo 提权。
// 行为与 Go 版 internal/update 对齐（产物命名/验签策略/交互确认/提权重试）。
use std::fs;
use std::io::Read;
use std::path::{Path, PathBuf};
use std::process::Command;

use ed25519_dalek::{Signature, Verifier, VerifyingKey};
use flate2::read::GzDecoder;

use crate::http::{HttpClient, Proxy};

// SIGN_PUB_KEY 是发布产物的 ed25519 公钥（base64），与 Go 版 internal/update/verify.go 相同。
const SIGN_PUB_KEY: &str = "YXpFqzZ8daPtFKwvpKTNoCkc8DIT2cG52cH3tvro9Go=";

pub struct Asset {
    pub name: String,
    pub url: String,
    pub sig_url: String,
}

pub struct Release {
    pub tag_name: String,
    pub body: String,
    pub assets: Vec<ReleaseAsset>,
}

pub struct ReleaseAsset {
    pub name: String,
    pub download_url: String,
}

pub struct Updater {
    pub repo: String,
    pub base_url: String,
    pub current: String,
    pub insecure: bool,
    pub exec_path: Option<PathBuf>,
    client: HttpClient,
}

impl Updater {
    pub fn new(proxy: Option<Proxy>) -> Self {
        Updater {
            repo: "29anan29/Fly-lang".to_string(),
            base_url: "https://api.github.com".to_string(),
            current: crate::version::string(),
            insecure: false,
            exec_path: None,
            client: HttpClient::new(proxy),
        }
    }

    pub fn latest(&self) -> Result<Release, String> {
        let url = format!("{}/repos/{}/releases/latest", self.base_url, self.repo);
        let resp = self
            .client
            .get(&url, &[("Accept", "application/vnd.github+json")])?;
        if resp.status != 200 {
            return Err(format!("检查更新失败: HTTP {}", resp.status));
        }
        parse_release(&resp.body)
    }

    pub fn asset_for(&self, os: &str, arch: &str, rel: &Release) -> Result<Asset, String> {
        let ext = if os == "windows" { "zip" } else { "tar.gz" };
        let want = format!("fly-{}-{}.{}", os, arch, ext);
        for a in &rel.assets {
            if a.name == want {
                let sig_url = rel
                    .assets
                    .iter()
                    .find(|x| x.name == format!("{}.sig", want))
                    .map(|x| x.download_url.clone())
                    .unwrap_or_default();
                return Ok(Asset {
                    name: a.name.clone(),
                    url: a.download_url.clone(),
                    sig_url,
                });
            }
        }
        Err(format!(
            "当前平台 {}/{} 暂无更新包（需要 {}）",
            os, arch, want
        ))
    }

    pub fn is_outdated(&self, latest: &str) -> bool {
        let cur = trim_tag(&self.current);
        let rel = trim_tag(latest);
        if cur == "dev" || cur.is_empty() {
            return true;
        }
        cur != rel
    }

    pub fn install(&self, a: &Asset, log: &mut dyn FnMut(&str)) -> Result<(), String> {
        log(&format!("下载 {}", a.name));
        let resp = self.client.get(&a.url, &[])?;
        if resp.status != 200 {
            return Err(format!("下载 {} 失败: HTTP {}", a.name, resp.status));
        }
        if !self.insecure {
            self.verify_asset(a, &resp.body, log)?;
        } else {
            log("跳过签名验证（--insecure，不推荐）");
        }

        let exe = self.executable()?;
        let exe_real = fs::canonicalize(&exe).unwrap_or(exe.clone());
        log("解包校验");
        let data = extract_binary(&resp.body, &a.name)?;
        let mut bin_name = exe_real
            .file_name()
            .map(|s| s.to_string_lossy().to_string())
            .unwrap_or_else(|| "fly".to_string());
        if std::env::consts::OS == "windows" && !bin_name.ends_with(".exe") {
            bin_name.push_str(".exe");
        }
        let dir = exe_real.parent().unwrap_or(Path::new("."));
        let new_path = dir.join(format!(".{}.new", bin_name));
        log(&format!("写入 {}", new_path.display()));
        fs::write(&new_path, &data).map_err(|e| format!("写入新版本失败: {}", e))?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            fs::set_permissions(&new_path, fs::Permissions::from_mode(0o755))
                .map_err(|e| format!("设置权限失败: {}", e))?;
        }
        log(&format!("原子替换 {}", exe_real.display()));
        fs::rename(&new_path, &exe_real).map_err(|e| {
            let _ = fs::remove_file(&new_path);
            if std::env::consts::OS == "windows" {
                format!(
                    "Windows 下无法替换正在运行的进程，请关闭后手动覆盖: {}",
                    exe_real.display()
                )
            } else {
                format!("替换可执行文件失败: {}", e)
            }
        })
    }

    fn verify_asset(&self, a: &Asset, data: &[u8], log: &mut dyn FnMut(&str)) -> Result<(), String> {
        if a.sig_url.is_empty() {
            return Err(format!(
                "缺少签名文件 {}.sig：为防篡改已拒绝安装，请从 GitHub Releases 手动下载",
                a.name
            ));
        }
        log(&format!("下载签名 {}.sig", a.name));
        let resp = self.client.get(&a.sig_url, &[])?;
        if resp.status != 200 {
            return Err(format!("下载签名失败: HTTP {}", resp.status));
        }
        log("验证签名");
        verify_signed(data, &resp.body).map_err(|e| format!("安全校验失败：{}", e))
    }

    pub fn check_writable(&self, dir: &Path) -> Result<(), String> {
        let probe = dir.join(format!(".fly-wtest-{}", std::process::id()));
        fs::OpenOptions::new()
            .create(true)
            .write(true)
            .open(&probe)
            .map_err(|e| format!("安装目录 {} 不可写: {}", dir.display(), e))?;
        let _ = fs::remove_file(&probe);
        Ok(())
    }

    pub fn executable(&self) -> Result<PathBuf, String> {
        if let Some(p) = &self.exec_path {
            return Ok(p.clone());
        }
        std::env::current_exe().map_err(|e| format!("无法定位当前可执行文件: {}", e))
    }

    pub fn sudo_retry(&self, args: &[String]) -> Result<(), String> {
        let exe = self.executable()?;
        let exe_real = fs::canonicalize(&exe).unwrap_or(exe);
        let status = Command::new("sudo")
            .arg(&exe_real)
            .arg("update")
            .args(args)
            .status()
            .map_err(|e| format!("sudo 执行失败: {}", e))?;
        if status.success() {
            Ok(())
        } else {
            Err(format!("sudo 更新失败（退出码 {:?}）", status.code()))
        }
    }
}

pub fn confirm() -> bool {
    let mut line = String::new();
    match std::io::stdin().read_line(&mut line) {
        Ok(_) => {
            let t = line.trim().to_lowercase();
            t == "y" || t == "yes"
        }
        Err(_) => false,
    }
}

fn trim_tag(tag: &str) -> String {
    tag.trim_start_matches('v').to_string()
}

// parse_release 极简 JSON 提取（GitHub Releases API 字段固定）：
// {"tag_name":"..","body":"..","assets":[{"name":"..","browser_download_url":".."},...]}
fn parse_release(body: &[u8]) -> Result<Release, String> {
    let s = String::from_utf8_lossy(body);
    let tag = json_string_field(&s, "tag_name").ok_or("响应缺少 tag_name")?;
    let body_txt = json_string_field(&s, "body").unwrap_or_default();
    let mut assets = Vec::new();
    if let Some(arr) = json_array(&s, "assets") {
        for item in split_json_objects(&arr) {
            if let Some(name) = json_string_field(item, "name") {
                let url = json_string_field(item, "browser_download_url").unwrap_or_default();
                assets.push(ReleaseAsset {
                    name,
                    download_url: url,
                });
            }
        }
    }
    Ok(Release {
        tag_name: tag,
        body: body_txt,
        assets,
    })
}

fn json_string_field(s: &str, key: &str) -> Option<String> {
    let pat = format!("\"{}\"", key);
    let idx = s.find(&pat)?;
    let rest = &s[idx + pat.len()..];
    let rest = rest.trim_start();
    let rest = rest.strip_prefix(':')?.trim_start();
    let rest = rest.strip_prefix('"')?;
    let mut out = String::new();
    let mut chars = rest.chars();
    while let Some(c) = chars.next() {
        match c {
            '"' => return Some(out),
            '\\' => match chars.next() {
                Some('n') => out.push('\n'),
                Some('t') => out.push('\t'),
                Some('r') => out.push('\r'),
                Some('u') => {
                    let hex: String = chars.by_ref().take(4).collect();
                    if let Ok(cp) = u32::from_str_radix(&hex, 16) {
                        if let Some(ch) = char::from_u32(cp) {
                            out.push(ch);
                        }
                    }
                }
                Some(other) => out.push(other),
                None => return None,
            },
            other => out.push(other),
        }
    }
    None
}

fn json_array<'a>(s: &'a str, key: &str) -> Option<&'a str> {
    let pat = format!("\"{}\"", key);
    let idx = s.find(&pat)?;
    let rest = &s[idx + pat.len()..];
    let rest = rest.trim_start().strip_prefix(':')?.trim_start();
    let inner = rest.strip_prefix('[')?;
    let mut depth = 0i32;
    let mut in_str = false;
    let mut esc = false;
    for (i, c) in inner.char_indices() {
        if in_str {
            if esc {
                esc = false;
            } else if c == '\\' {
                esc = true;
            } else if c == '"' {
                in_str = false;
            }
            continue;
        }
        match c {
            '"' => in_str = true,
            '[' => depth += 1,
            ']' => {
                if depth == 0 {
                    return Some(&inner[..i]);
                }
                depth -= 1;
            }
            _ => {}
        }
    }
    None
}

fn split_json_objects(arr: &str) -> Vec<&str> {
    let mut out = Vec::new();
    let mut depth = 0i32;
    let mut start = 0usize;
    let mut in_str = false;
    let mut esc = false;
    for (i, c) in arr.char_indices() {
        if in_str {
            if esc {
                esc = false;
            } else if c == '\\' {
                esc = true;
            } else if c == '"' {
                in_str = false;
            }
            continue;
        }
        match c {
            '"' => in_str = true,
            '{' => {
                if depth == 0 {
                    start = i;
                }
                depth += 1;
            }
            '}' => {
                depth -= 1;
                if depth == 0 {
                    out.push(&arr[start..=i]);
                }
            }
            _ => {}
        }
    }
    out
}

// verify_signed 校验 data 的 ed25519 签名（sig 为 64 字节原始签名）。
pub fn verify_signed(data: &[u8], sig: &[u8]) -> Result<(), String> {
    use base64_engine;
    let pub_raw = base64_engine::decode(SIGN_PUB_KEY).map_err(|e| format!("内嵌公钥非法: {}", e))?;
    if sig.len() != 64 {
        return Err(format!("签名长度 {} 非法（应为 64）", sig.len()));
    }
    let key = VerifyingKey::from_bytes(pub_raw.as_slice().try_into().map_err(|_| "内嵌公钥长度非法".to_string())?)
            .map_err(|e| format!("内嵌公钥非法: {}", e))?;
    let sig = Signature::from_bytes(sig.try_into().unwrap());
    key.verify(data, &sig)
        .map_err(|_| "签名验证失败：产物可能被篡改或来源不可信".to_string())
}

// extract_binary 从 tar.gz/zip 安装包中提取可执行文件（fly/fly.exe）。
fn extract_binary(archive: &[u8], name: &str) -> Result<Vec<u8>, String> {
    if name.ends_with(".zip") {
        let reader = std::io::Cursor::new(archive);
        let mut z = zip::ZipArchive::new(reader).map_err(|e| format!("zip 解析失败: {}", e))?;
        for i in 0..z.len() {
            let mut f = z.by_index(i).map_err(|e| format!("zip 读取失败: {}", e))?;
            let fname = f.name().to_string();
            if is_binary_name(&fname) {
                let mut data = Vec::new();
                f.read_to_end(&mut data).map_err(|e| format!("zip 解压失败: {}", e))?;
                return Ok(data);
            }
        }
        return Err("安装包内未找到可执行文件".to_string());
    }
    let gz = GzDecoder::new(archive);
    let mut tar = tar_reader(gz);
    loop {
        let header = read_tar_header(&mut tar)?;
        if header.is_none() {
            break;
        }
        let h = header.unwrap();
        if h.is_file && is_binary_name(&h.name) {
            let mut data = vec![0u8; h.size];
            tar.read_exact(&mut data).map_err(|e| format!("tar 解压失败: {}", e))?;
            return Ok(data);
        }
        skip_tar_padding(&mut tar, h.size)?;
    }
    Err("安装包内未找到可执行文件".to_string())
}

struct TarHeader {
    name: String,
    size: usize,
    is_file: bool,
}

fn read_tar_header(r: &mut impl Read) -> Result<Option<TarHeader>, String> {
    let mut block = [0u8; 512];
    match r.read_exact(&mut block) {
        Ok(()) => {}
        Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => return Ok(None),
        Err(e) => return Err(format!("tar 读取失败: {}", e)),
    }
    if block.iter().all(|&b| b == 0) {
        return Ok(None);
    }
    let name = String::from_utf8_lossy(&block[..100])
        .trim_end_matches('\0')
        .to_string();
    let size_field = String::from_utf8_lossy(&block[124..136]);
    let size = usize::from_str_radix(size_field.trim_end_matches('\0').trim(), 8)
        .map_err(|_| "tar 大小字段非法")?;
    let typeflag = block[156];
    let is_file = typeflag == b'0' || typeflag == 0 || (typeflag == b'\0');
    Ok(Some(TarHeader { name, size, is_file }))
}

fn skip_tar_padding(r: &mut impl Read, size: usize) -> Result<(), String> {
    let pad = (512 - (size % 512)) % 512;
    let mut buf = vec![0u8; pad];
    r.read_exact(&mut buf).map_err(|e| format!("tar 填充读取失败: {}", e))
}

fn tar_reader(r: impl Read) -> impl Read {
    r
}

fn is_binary_name(name: &str) -> bool {
    let base = name.rsplit('/').next().unwrap_or(name);
    base == "fly" || base == "fly.exe"
}

mod base64_engine {
    // 手写 base64 解码（std-only）：产物公钥长度 32 字节。
    pub fn decode(s: &str) -> Result<Vec<u8>, String> {
        const TBL: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
        let mut out = Vec::new();
        let mut buf = 0u32;
        let mut bits = 0u32;
        for c in s.bytes() {
            if c == b'=' {
                break;
            }
            let v = TBL
                .iter()
                .position(|&t| t == c)
                .ok_or("base64 字符非法")? as u32;
            buf = (buf << 6) | v;
            bits += 6;
            if bits >= 8 {
                bits -= 8;
                out.push((buf >> bits) as u8);
            }
        }
        Ok(out)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn base64_roundtrip() {
        let raw = base64_engine::decode("YXpFqzZ8daPtFKwvpKTNoCkc8DIT2cG52cH3tvro9Go=").unwrap();
        assert_eq!(raw.len(), 32);
    }

    #[test]
    fn parse_release_ok() {
        let body = br#"{"tag_name":"v1.2.3","body":"notes","assets":[{"name":"fly-linux-x86_64.tar.gz","browser_download_url":"https://x/fly-linux-x86_64.tar.gz"},{"name":"fly-linux-x86_64.tar.gz.sig","browser_download_url":"https://x/s"}]}"#;
        let r = parse_release(body).unwrap();
        assert_eq!(r.tag_name, "v1.2.3");
        assert_eq!(r.assets.len(), 2);
        assert_eq!(r.assets[0].name, "fly-linux-x86_64.tar.gz");
    }

    #[test]
    fn asset_for_linux() {
        let u = Updater::new(None);
        let rel = Release {
            tag_name: "v1".into(),
            body: String::new(),
            assets: vec![
                ReleaseAsset {
                    name: "fly-linux-x86_64.tar.gz".into(),
                    download_url: "https://x/a".into(),
                },
                ReleaseAsset {
                    name: "fly-linux-x86_64.tar.gz.sig".into(),
                    download_url: "https://x/s".into(),
                },
            ],
        };
        let a = u.asset_for("linux", "x86_64", &rel).unwrap();
        assert_eq!(a.name, "fly-linux-x86_64.tar.gz");
        assert_eq!(a.sig_url, "https://x/s");
    }

    #[test]
    fn asset_for_windows() {
        let u = Updater::new(None);
        let rel = Release {
            tag_name: "v1".into(),
            body: String::new(),
            assets: vec![ReleaseAsset {
                name: "fly-windows-x86_64.zip".into(),
                download_url: "https://x/z".into(),
            }],
        };
        let a = u.asset_for("windows", "x86_64", &rel).unwrap();
        assert_eq!(a.name, "fly-windows-x86_64.zip");
        assert!(a.sig_url.is_empty());
    }

    #[test]
    fn outdated_logic() {
        assert!(Updater::new(None).is_outdated("v1.2.3"));
    }

    #[test]
    fn verify_signed_rejects_bad() {
        let r = verify_signed(b"data", &[0u8; 64]);
        assert!(r.is_err());
    }
}