// checkd.rs：编译检查守护进程——stdio 二进制帧协议，桥接 Go 版 fly-checkd（checker 留 Go）。
use std::io::{Read, Write};
use std::path::PathBuf;
use std::process::{Command, Stdio};

use crate::diagnostic::{Diagnostic, Position};

// 与 Go 侧 cmd/fly-checkd 的二进制帧协议完全对应。
// 请求: [4B BE len][1B color][4B BE path_len][path][4B BE src_len][src]
// 响应: [4B BE len][1B status] 0x00 → [1B count] 每条
//          [4B BE code_len][code][4B LE line][4B LE col][4B BE msg_len][msg]
//        0x01 → [4B BE msg_len][msg]（checkd 内部错误）

pub struct CheckdResult {
    pub diags: Vec<Diagnostic>,
    pub server_error: Option<String>,
}

pub fn find_checkd() -> Option<PathBuf> {
    if let Ok(p) = std::env::var("FLY_CHECKD") {
        if !p.is_empty() {
            return Some(PathBuf::from(p));
        }
    }
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            let cand = dir.join("fly-checkd");
            if cand.exists() {
                return Some(cand);
            }
        }
    }
    find_on_path()
}

fn find_on_path() -> Option<PathBuf> {
    let path = std::env::var("PATH").ok()?;
    for dir in path.split(':') {
        let cand = PathBuf::from(dir).join("fly-checkd");
        if cand.exists() {
            return Some(cand);
        }
    }
    None
}

pub fn check_src(
    checkd: &PathBuf,
    src: &str,
    path: &str,
    color: bool,
) -> Result<CheckdResult, String> {
    let mut child = Command::new(checkd)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .map_err(|e| format!("启动 checkd 失败: {}", e))?;

    let mut payload = Vec::new();
    payload.push(color as u8);
    push_be32(&mut payload, path.len() as u32);
    payload.extend_from_slice(path.as_bytes());
    push_be32(&mut payload, src.len() as u32);
    payload.extend_from_slice(src.as_bytes());

    let mut frame = Vec::new();
    push_be32(&mut frame, payload.len() as u32);
    frame.extend_from_slice(&payload);

    if let Some(mut stdin) = child.stdin.take() {
        stdin
            .write_all(&frame)
            .and_then(|_| stdin.flush())
            .map_err(|e| format!("写入 checkd 失败: {}", e))?;
    }
    drop(child.stdin.take());

    let mut stdout = Vec::new();
    child
        .stdout
        .take()
        .ok_or("checkd stdout 不可用")?
        .read_to_end(&mut stdout)
        .map_err(|e| format!("读取 checkd 失败: {}", e))?;

    let status = child
        .wait()
        .map_err(|e| format!("等待 checkd 失败: {}", e))?;
    if !status.success() {
        return Err(format!("checkd 退出码 {}", status.code().unwrap_or(-1)));
    }

    let bytes = stdout.as_slice();
    if bytes.len() < 4 {
        return Err("checkd 响应过短".to_string());
    }
    let len = be32(&bytes[0..4]) as usize;
    if bytes.len() < 4 + len {
        return Err("checkd 响应长度不符".to_string());
    }
    parse_response(&bytes[4..4 + len])
}

// CheckdSession 长驻 checkd 进程：复用单进程处理多帧请求（LSP 场景，消除每次 spawn 开销）。
pub struct CheckdSession {
    _child: std::process::Child,
    stdin: std::process::ChildStdin,
    stdout: std::process::ChildStdout,
}

impl CheckdSession {
    pub fn spawn(checkd: &PathBuf) -> Result<CheckdSession, String> {
        let mut child = Command::new(checkd)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::null())
            .spawn()
            .map_err(|e| format!("启动 checkd 失败: {}", e))?;
        let stdin = child.stdin.take().ok_or("checkd stdin 不可用")?;
        let stdout = child.stdout.take().ok_or("checkd stdout 不可用")?;
        Ok(CheckdSession {
            _child: child,
            stdin,
            stdout,
        })
    }

    pub fn check(&mut self, src: &str, path: &str, color: bool) -> Result<CheckdResult, String> {
        let mut payload = Vec::new();
        payload.push(color as u8);
        push_be32(&mut payload, path.len() as u32);
        payload.extend_from_slice(path.as_bytes());
        push_be32(&mut payload, src.len() as u32);
        payload.extend_from_slice(src.as_bytes());

        let mut frame = Vec::new();
        push_be32(&mut frame, payload.len() as u32);
        frame.extend_from_slice(&payload);
        self.stdin
            .write_all(&frame)
            .and_then(|_| self.stdin.flush())
            .map_err(|e| format!("写入 checkd 失败: {}", e))?;

        let mut len_buf = [0u8; 4];
        self.stdout
            .read_exact(&mut len_buf)
            .map_err(|e| format!("读取 checkd 失败: {}", e))?;
        let len = be32(&len_buf) as usize;
        let mut body = vec![0u8; len];
        self.stdout
            .read_exact(&mut body)
            .map_err(|e| format!("读取 checkd 失败: {}", e))?;
        parse_response(&body)
    }
}

pub fn parse_response(body: &[u8]) -> Result<CheckdResult, String> {
    if body.is_empty() {
        return Err("checkd 响应为空".to_string());
    }
    if body[0] == 0x01 {
        let msg = read_str(body, 1).unwrap_or_else(|_| "checkd 内部错误".to_string());
        return Ok(CheckdResult {
            diags: Vec::new(),
            server_error: Some(msg),
        });
    }
    if body[0] != 0x00 {
        return Err(format!("checkd 未知状态 {}", body[0]));
    }
    let count = be32(&body[1..5]) as usize;
    let mut off = 5usize;
    let mut diags = Vec::with_capacity(count);
    for _ in 0..count {
        let code = read_str(body, off)?;
        off += 4 + code.len();
        if off + 8 > body.len() {
            return Err("checkd 诊断字段截断".to_string());
        }
        let line = le32(&body[off..off + 4]);
        let col = le32(&body[off + 4..off + 8]);
        off += 8;
        let msg = read_str(body, off)?;
        off += 4 + msg.len();
        diags.push(Diagnostic {
            pos: Position { line, col },
            code,
            msg,
        });
    }
    Ok(CheckdResult {
        diags,
        server_error: None,
    })
}

fn read_str(body: &[u8], off: usize) -> Result<String, String> {
    if off + 4 > body.len() {
        return Err("checkd 字段长度截断".to_string());
    }
    let l = be32(&body[off..off + 4]) as usize;
    if off + 4 + l > body.len() {
        return Err("checkd 字段截断".to_string());
    }
    String::from_utf8(body[off + 4..off + 4 + l].to_vec())
        .map_err(|_| "checkd 非 UTF-8".to_string())
        .map(|s| s )
}

fn push_be32(v: &mut Vec<u8>, n: u32) {
    v.extend_from_slice(&n.to_be_bytes());
}

fn be32(b: &[u8]) -> u32 {
    u32::from_be_bytes([b[0], b[1], b[2], b[3]])
}

fn le32(b: &[u8]) -> u32 {
    u32::from_le_bytes([b[0], b[1], b[2], b[3]])
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn frame_roundtrip_encode() {
        let mut v = Vec::new();
        push_be32(&mut v, 300);
        assert_eq!(be32(&v), 300);
        let mut w = Vec::new();
        push_be32(&mut w, 7);
        assert_eq!(be32(&w), 7);
    }

    #[test]
    fn checkd_reachable() {
        let Some(checkd) = find_checkd() else {
            return;
        };
        let r = check_src(&checkd, "x = (1 +\n", "t.fly", false).expect("checkd 调用");
        assert!(r.server_error.is_none());
        assert_eq!(r.diags.len(), 1);
        assert_eq!(r.diags[0].code, "E0002");
    }
}
