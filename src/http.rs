// http.rs：极简 HTTP/1.1 客户端——rustls TLS + 手写 TCP/SOCKS5/代理 CONNECT，零系统依赖。
// 支持 https://（直连）、http:// https://（CONNECT 隧道）、socks5://[user:pass@]host:port。
use std::io::{Read, Write};
use std::net::TcpStream;
use std::sync::Arc;
use std::time::Duration;

use rustls::pki_types::ServerName;
use rustls::{ClientConfig, ClientConnection, StreamOwned};

pub enum Proxy {
    Http(String),
    Https(String),
    Socks5(String),
}

pub struct HttpClient {
    proxy: Option<Proxy>,
}

pub struct Response {
    pub status: u16,
    pub headers: Vec<(String, String)>,
    pub body: Vec<u8>,
}

impl Response {
    pub fn header(&self, name: &str) -> Option<&str> {
        self.headers
            .iter()
            .find(|(k, _)| k.eq_ignore_ascii_case(name))
            .map(|(_, v)| v.as_str())
    }
}

fn parse_url(url: &str) -> (String, u16, String, String) {
    let rest = url
        .strip_prefix("https://")
        .expect("仅支持 https:// URL");
    let (hostport, path) = match rest.find('/') {
        Some(i) => (&rest[..i], &rest[i..]),
        None => (rest, "/"),
    };
    let (host, port) = match hostport.rsplit_once(':') {
        Some((h, p)) if p.chars().all(|c| c.is_ascii_digit()) => {
            (h, p.parse::<u16>().unwrap_or(443))
        }
        _ => (hostport, 443),
    };
    (host.to_string(), port, path.to_string(), url.to_string())
}

// resolve_redirect 拼接重定向 Location：绝对 https 原样；相对路径基于当前
// URL 的 scheme/host/目录解析；http 明文拒绝（防降级）。
fn resolve_redirect(cur: &str, loc: &str) -> Result<String, String> {
    if loc.starts_with("https://") {
        return Ok(loc.to_string());
    }
    if loc.starts_with("http://") {
        return Err("拒绝 HTTP 明文重定向".to_string());
    }
    let (h, p, path, _) = parse_url(cur);
    let prefix = format!("https://{}:{}", h, p);
    if loc.starts_with('/') {
        Ok(format!("{}{}", prefix, loc))
    } else {
        let dir = path.rsplit_once('/').map(|(d, _)| d.to_string()).unwrap_or_default();
        Ok(format!("{}{}/{}", prefix, dir, loc))
    }
}

impl HttpClient {
    pub fn new(proxy: Option<Proxy>) -> Self {
        HttpClient { proxy }
    }

    // tcp_connect 建立原始 TCP 连接（直连或经代理隧道）。
    fn tcp_connect(&self, host: &str, port: u16) -> Result<TcpStream, String> {
        let addr = format!("{}:{}", host, port);
        match &self.proxy {
            None => TcpStream::connect(&addr).map_err(|e| format!("连接 {} 失败: {}", addr, e)),
            Some(Proxy::Socks5(u)) => socks5_connect(u, &addr),
            Some(Proxy::Http(u)) | Some(Proxy::Https(u)) => {
                let (phost, pport, _) = parse_proxy(u);
                let pa = format!("{}:{}", phost, pport);
                let mut s = TcpStream::connect(&pa)
                    .map_err(|e| format!("连接代理 {} 失败: {}", pa, e))?;
                let _ = s.set_read_timeout(Some(Duration::from_secs(300)));
                let _ = s.set_write_timeout(Some(Duration::from_secs(60)));
                let req = format!(
                    "CONNECT {}:{} HTTP/1.1\r\nHost: {}:{}\r\n\r\n",
                    host, port, host, port
                );
                s.write_all(req.as_bytes())
                    .map_err(|e| format!("CONNECT 请求失败: {}", e))?;
                let mut buf = [0u8; 4096];
                let mut resp = Vec::new();
                loop {
                    let n = s.read(&mut buf).map_err(|e| format!("读取代理响应失败: {}", e))?;
                    if n == 0 {
                        return Err("代理连接中断".to_string());
                    }
                    resp.extend_from_slice(&buf[..n]);
                    if resp.windows(4).any(|w| w == b"\r\n\r\n") {
                        break;
                    }
                }
                let head = String::from_utf8_lossy(&resp);
                let status = head
                    .split(' ')
                    .nth(1)
                    .and_then(|s| s.parse::<u16>().ok())
                    .unwrap_or(0);
                if status != 200 {
                    return Err(format!("代理 CONNECT 失败: HTTP {}", status));
                }
                Ok(s)
            }
        }
    }

    // tls_stream 在 TCP 流上建立 TLS，返回 rustls 读写流。
    fn tls_stream(&self, host: &str, tcp: TcpStream) -> Result<StreamOwned<ClientConnection, TcpStream>, String> {
        let mut roots = rustls::RootCertStore::empty();
        roots.extend(webpki_roots::TLS_SERVER_ROOTS.iter().cloned());
        let config = ClientConfig::builder_with_provider(Arc::new(rustls::crypto::ring::default_provider()))
            .with_safe_default_protocol_versions()
            .map_err(|e| format!("TLS 配置失败: {}", e))?
            .with_root_certificates(roots)
            .with_no_client_auth();
        let name = ServerName::try_from(host.to_string())
            .map_err(|e| format!("TLS 主机名非法: {}", e))?;
        let conn = ClientConnection::new(Arc::new(config), name)
            .map_err(|e| format!("TLS 握手初始化失败: {}", e))?;
        Ok(StreamOwned::new(conn, tcp))
    }

    // get 发起 GET 请求，自动跟随重定向（最多 10 跳），返回最终响应。
    pub fn get(&self, url: &str, headers: &[(&str, &str)]) -> Result<Response, String> {
        let mut cur = url.to_string();
        for _ in 0..10 {
            let (host, port, path, _) = parse_url(&cur);
            let tcp = self.tcp_connect(&host, port)?;
            let _ = tcp.set_read_timeout(Some(Duration::from_secs(300)));
            let _ = tcp.set_write_timeout(Some(Duration::from_secs(60)));
            let mut s = self.tls_stream(&host, tcp)?;
            let mut req = format!("GET {} HTTP/1.1\r\nHost: {}\r\nUser-Agent: pyfly-lang/update\r\nAccept: */*\r\n", path, host);
            for (k, v) in headers {
                req.push_str(&format!("{}: {}\r\n", k, v));
            }
            req.push_str("Connection: close\r\n\r\n");
            s.write_all(req.as_bytes())
                .map_err(|e| format!("发送请求失败: {}", e))?;
            s.flush().ok();
            let mut raw = Vec::new();
            let mut buf = [0u8; 16384];
            loop {
                match s.read(&mut buf) {
                    Ok(0) => break,
                    Ok(n) => raw.extend_from_slice(&buf[..n]),
                    Err(e) => return Err(format!("读取响应失败: {}", e)),
                }
            }
            let resp = parse_http_response(&raw)?;
            if (300..400).contains(&resp.status) {
                if let Some(loc) = resp.header("location") {
                    cur = resolve_redirect(&cur, loc)?;
                    continue;
                }
                return Err(format!("重定向响应缺少 Location: HTTP {}", resp.status));
            }
            return Ok(resp);
        }
        Err("重定向次数过多".to_string())
    }
}

fn parse_proxy(u: &str) -> (String, u16, String) {
    let (rest, scheme) = if let Some(r) = u.strip_prefix("http://") {
        (r, "http")
    } else if let Some(r) = u.strip_prefix("https://") {
        (r, "https")
    } else {
        (u, "http")
    };
    let hostport = rest.split('/').next().unwrap_or(rest);
    let (host, port) = match hostport.rsplit_once(':') {
        Some((h, p)) if p.chars().all(|c| c.is_ascii_digit()) => {
            (h.to_string(), p.parse::<u16>().unwrap_or(80))
        }
        _ => (hostport.to_string(), if scheme == "https" { 443 } else { 80 }),
    };
    (host, port, scheme.to_string())
}

// socks5_connect 手写 SOCKS5 握手（无认证/用户名密码），返回已建立的 TCP 流。
fn socks5_connect(proxy: &str, target: &str) -> Result<TcpStream, String> {
    let rest = proxy
        .strip_prefix("socks5://")
        .or_else(|| proxy.strip_prefix("socks5h://"))
        .ok_or("SOCKS5 代理格式应为 socks5://[user:pass@]host:port")?;
    let (userinfo, hostport) = match rest.rsplit_once('@') {
        Some((u, h)) => (Some(u.to_string()), h.to_string()),
        None => (None, rest.to_string()),
    };
    let (phost, pport) = match hostport.rsplit_once(':') {
        Some((h, p)) => (h.to_string(), p.parse::<u16>().map_err(|_| "代理端口非法")?),
        None => return Err("代理缺少端口".to_string()),
    };
    let pa = format!("{}:{}", phost, pport);
    let mut s = TcpStream::connect(&pa).map_err(|e| format!("连接代理 {} 失败: {}", pa, e))?;
    let _ = s.set_read_timeout(Some(Duration::from_secs(300)));
    let _ = s.set_write_timeout(Some(Duration::from_secs(60)));

    let (user, pass) = match &userinfo {
        Some(u) => match u.rsplit_once(':') {
            Some((a, b)) => (a.to_string(), b.to_string()),
            None => (u.clone(), String::new()),
        },
        None => (String::new(), String::new()),
    };
    let methods: Vec<u8> = if user.is_empty() { vec![0x00] } else { vec![0x00, 0x02] };
    s.write_all(&[0x05, methods.len() as u8])
        .and_then(|_| s.write_all(&methods))
        .map_err(|e| format!("SOCKS5 握手失败: {}", e))?;
    let mut resp = [0u8; 2];
    s.read_exact(&mut resp).map_err(|e| format!("SOCKS5 握手响应失败: {}", e))?;
    if resp[0] != 0x05 {
        return Err("代理返回非 SOCKS5 协议".to_string());
    }
    match resp[1] {
        0xFF => return Err("代理无可用认证方式".to_string()),
        0x02 => {
            s.write_all(&[0x01, user.len() as u8])
                .and_then(|_| s.write_all(user.as_bytes()))
                .and_then(|_| s.write_all(&[pass.len() as u8]))
                .and_then(|_| s.write_all(pass.as_bytes()))
                .map_err(|e| format!("SOCKS5 认证失败: {}", e))?;
            s.read_exact(&mut resp).map_err(|e| format!("SOCKS5 认证响应失败: {}", e))?;
            if resp[1] != 0x00 {
                return Err("SOCKS5 用户名密码认证失败".to_string());
            }
        }
        0x00 => {}
        other => return Err(format!("代理选择了未知认证方式 0x{:02X}", other)),
    }

    let (host, port) = match target.rsplit_once(':') {
        Some((h, p)) => (
            h.to_string(),
            p.parse::<u16>().map_err(|_| "目标端口非法")?,
        ),
        None => return Err("目标地址缺少端口".to_string()),
    };
    let mut req = vec![0x05, 0x01, 0x00];
    if let Ok(ip) = host.parse::<std::net::IpAddr>() {
        match ip {
            std::net::IpAddr::V4(v4) => {
                req.push(0x01);
                req.extend_from_slice(&v4.octets());
            }
            std::net::IpAddr::V6(v6) => {
                req.push(0x04);
                req.extend_from_slice(&v6.octets());
            }
        }
    } else {
        req.push(0x03);
        req.push(host.len() as u8);
        req.extend_from_slice(host.as_bytes());
    }
    req.extend_from_slice(&port.to_be_bytes());
    s.write_all(&req).map_err(|e| format!("SOCKS5 连接请求失败: {}", e))?;
    let mut head = [0u8; 4];
    s.read_exact(&mut head).map_err(|e| format!("SOCKS5 响应失败: {}", e))?;
    if head[0] != 0x05 {
        return Err("SOCKS5 请求响应异常".to_string());
    }
    if head[1] != 0x00 {
        return Err(format!("SOCKS5 连接失败，错误码 0x{:02X}", head[1]));
    }
    let skip = match head[3] {
        0x01 => 4 + 2,
        0x04 => 16 + 2,
        _ => 1 + 2,
    };
    let mut junk = vec![0u8; skip];
    s.read_exact(&mut junk).map_err(|e| format!("SOCKS5 地址解析失败: {}", e))?;
    Ok(s)
}

// parse_http_response 解析原始 HTTP/1.1 响应（Content-Length 或 chunked 或 EOF 截止）。
fn parse_http_response(raw: &[u8]) -> Result<Response, String> {
    let head_end = raw
        .windows(4)
        .position(|w| w == b"\r\n\r\n")
        .ok_or("响应头不完整")?;
    let head = String::from_utf8_lossy(&raw[..head_end]);
    let mut lines = head.split("\r\n");
    let status_line = lines.next().unwrap_or("");
    let mut parts = status_line.split(' ');
    let _ = parts.next();
    let status = parts
        .next()
        .and_then(|s| s.parse::<u16>().ok())
        .ok_or("响应状态行非法")?;
    let mut headers = Vec::new();
    for l in lines {
        if let Some((k, v)) = l.split_once(':') {
            headers.push((k.trim().to_string(), v.trim().to_string()));
        }
    }
    let body_start = head_end + 4;
    let body = if let Some(cl) = headers
        .iter()
        .find(|(k, _)| k.eq_ignore_ascii_case("content-length"))
    {
        let n = cl
            .1
            .parse::<usize>()
            .map_err(|_| "Content-Length 非法")?;
        raw.get(body_start..body_start + n)
            .ok_or("响应体不完整")?
            .to_vec()
    } else if headers
        .iter()
        .any(|(k, v)| k.eq_ignore_ascii_case("transfer-encoding") && v.contains("chunked"))
    {
        let mut out = Vec::new();
        let mut rest = &raw[body_start..];
        loop {
            let line_end = rest
                .windows(2)
                .position(|w| w == b"\r\n")
                .ok_or("chunked 块长度行不完整")?;
            let size = usize::from_str_radix(
                String::from_utf8_lossy(&rest[..line_end]).trim(),
                16,
            )
            .map_err(|_| "chunked 块长度非法")?;
            let chunk_start = line_end + 2;
            if size == 0 {
                break;
            }
            let chunk = rest
                .get(chunk_start..chunk_start + size)
                .ok_or("chunked 数据不完整")?;
            out.extend_from_slice(chunk);
            rest = &rest[chunk_start + size + 2..];
        }
        out
    } else {
        raw[body_start..].to_vec()
    };
    Ok(Response {
        status,
        headers,
        body,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_http_basic() {
        let raw = b"HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello";
        let r = parse_http_response(raw).unwrap();
        assert_eq!(r.status, 200);
        assert_eq!(r.body, b"hello");
    }

    #[test]
    fn parse_http_chunked() {
        let raw = b"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n";
        let r = parse_http_response(raw).unwrap();
        assert_eq!(r.body, b"hello world");
    }

    #[test]
    fn parse_http_redirect() {
        let raw = b"HTTP/1.1 302 Found\r\nLocation: https://example.com/x\r\n\r\n";
        let r = parse_http_response(raw).unwrap();
        assert_eq!(r.status, 302);
        assert_eq!(r.header("location"), Some("https://example.com/x"));
    }

    #[test]
    fn resolve_redirect_relative() {
        assert_eq!(
            resolve_redirect("https://a.com/v1/api", "next").unwrap(),
            "https://a.com:443/v1/next"
        );
        assert_eq!(
            resolve_redirect("https://a.com/v1/api", "/root").unwrap(),
            "https://a.com:443/root"
        );
        assert_eq!(
            resolve_redirect("https://a.com", "x").unwrap(),
            "https://a.com:443/x"
        );
        assert_eq!(
            resolve_redirect("https://a.com/p", "https://b.com/z").unwrap(),
            "https://b.com/z"
        );
        assert!(resolve_redirect("https://a.com/p", "http://b.com/z").is_err());
    }
}