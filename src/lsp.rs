// lsp.rs：LSP 服务器——JSON-RPC 2.0 over stdio（Content-Length 帧），诊断走 checkd 长驻进程。
// 行为与 Go 版 internal/lsp 对齐：initialize/initialized/shutdown/exit、didOpen/didChange(full)/didSave/didClose、
// publishDiagnostics、hover（8 关键字）、fly/forceCheck。
use std::collections::HashMap;
use std::io::{BufRead, Read, Write};
use std::path::PathBuf;
use std::sync::mpsc;
use std::sync::{Arc, Mutex};

use crate::checkd::{self, CheckdSession};
use crate::json::Json;

const KEYWORD_DOCS: &[(&str, &str)] = &[
    ("safe", "**safe** · 强制净化污点变量\n\n声明此变量为污点源，必须经净化后才能流入危险操作（eval/exec/os.system/SQL 等）。编译期污点追踪，零运行时残留。"),
    ("only", "**only** · 白名单权限块\n\n块内只允许白名单内的模块/函数调用（如 `only (sys):`）。编译期校验 + `__builtins__` 白名单代理。"),
    ("lock", "**lock** · 锁定常量\n\n锁定变量为常量：禁止再赋值、AugAssign、setattr 与 `globals()['X']` 反射读取。编译期符号表拦截。"),
    ("mask", "**mask** · 遮蔽敏感数据\n\n标记敏感变量（密码/token），禁止流入 print/logging/f-string 等输出上下文。编译期检测。"),
    ("cage", "**cage** · 资源约束\n\n`cage(max_time=, max_memory=):` 限制代码块执行时间与内存，超限抛 `TimeoutError`/`ResourceExhaustedError`。运行时 signal/resource。"),
    ("guard", "**guard** · 强制输入验证\n\n`guard x: type, 条件` 展开为断言，不满足抛 `GuardError`。编译期验证 + 生成断言。"),
    ("seal", "**seal** · 冻结对象\n\n`seal class` 冻结类与实例，禁止增删改属性。类体注入 `__setattr__` 拦截。"),
    ("trace", "**trace** · 审计日志\n\n`trace(level=, args=, ret=)` 在函数入口/出口插入 logging 调用，记录参数与返回值。"),
];

const DEBOUNCE_MS: u64 = 120;

struct Doc {
    text: String,
    version: u64,
}

pub struct Server {
    docs: Mutex<HashMap<String, Doc>>,
    checkd: Mutex<Option<CheckdSession>>,
    checkd_path: PathBuf,
    shut_ok: Mutex<bool>,
    out: Mutex<Option<mpsc::Sender<Vec<u8>>>>,
}

impl Server {
    pub fn new() -> Result<Server, String> {
        let path = checkd::find_checkd().ok_or("找不到 fly-checkd（设置 FLY_CHECKD 环境变量指定路径）")?;
        Ok(Server {
            docs: Mutex::new(HashMap::new()),
            checkd: Mutex::new(None),
            checkd_path: path,
            shut_ok: Mutex::new(false),
            out: Mutex::new(None),
        })
    }

    pub fn run(self: Arc<Server>, stdin: &mut impl Read) -> Result<(), String> {
        let (tx, rx) = mpsc::channel::<Vec<u8>>();
        *self.out.lock().unwrap() = Some(tx.clone());
        let writer = std::thread::spawn(move || {
            let mut w = std::io::stdout();
            for msg in rx {
                if msg.is_empty() {
                    break;
                }
                if write_message(&mut w, &msg).is_err() {
                    break;
                }
                let _ = w.flush();
            }
        });

        let mut reader = std::io::BufReader::new(stdin);
        loop {
            let msg = match read_message(&mut reader) {
                Ok(m) => m,
                Err(e) => {
                    let _ = tx.send(Vec::new());
                    writer.join().ok();
                    return Err(e);
                }
            };
            let parsed = Json::parse(&msg);
            let (id, method) = match &parsed {
                Ok(j) => (
                    j.get("id").cloned(),
                    j.get("method").and_then(|m| m.as_str()).unwrap_or("").to_string(),
                ),
                Err(e) => {
                    eprintln!("lsp: JSON 解析失败: {}", e);
                    continue;
                }
            };
            let params = parsed
                .as_ref()
                .ok()
                .and_then(|j| j.get("params").cloned())
                .unwrap_or(Json::Null);
            let resp = self.dispatch(&method, id.as_ref(), &params);
            if let Some(r) = resp {
                let _ = tx.send(r.encode().into_bytes());
            }
            if method == "exit" {
                let _ = tx.send(Vec::new());
                writer.join().ok();
                if *self.shut_ok.lock().unwrap() {
                    return Ok(());
                }
                return Err("exit 未先 shutdown".to_string());
            }
        }
    }

    fn session(&self) -> Result<std::sync::MutexGuard<'_, Option<CheckdSession>>, String> {
        let mut g = self.checkd.lock().unwrap();
        if g.is_none() {
            *g = Some(CheckdSession::spawn(&self.checkd_path)?);
        }
        Ok(g)
    }

    fn dispatch(self: &Arc<Server>, method: &str, id: Option<&Json>, params: &Json) -> Option<Json> {
        let rpc = |result: Json| {
            Some(Json::Obj(vec![
                ("jsonrpc".to_string(), Json::Str("2.0".to_string())),
                ("id".to_string(), id.cloned().unwrap_or(Json::Null)),
                ("result".to_string(), result),
            ]))
        };
        let rpc_err = |code: i64, message: String| {
            Some(Json::Obj(vec![
                ("jsonrpc".to_string(), Json::Str("2.0".to_string())),
                ("id".to_string(), id.cloned().unwrap_or(Json::Null)),
                (
                    "error".to_string(),
                    Json::Obj(vec![
                        ("code".to_string(), Json::Num(code as f64)),
                        ("message".to_string(), Json::Str(message)),
                    ]),
                ),
            ]))
        };
        match method {
            "initialize" => rpc(Json::Obj(vec![
                (
                    "capabilities".to_string(),
                    Json::Obj(vec![
                        (
                            "textDocumentSync".to_string(),
                            Json::Obj(vec![
                                ("openClose".to_string(), Json::Bool(true)),
                                ("change".to_string(), Json::Num(1.0)),
                                ("save".to_string(), Json::Bool(true)),
                            ]),
                        ),
                        ("hoverProvider".to_string(), Json::Bool(true)),
                    ]),
                ),
                (
                    "serverInfo".to_string(),
                    Json::Obj(vec![
                        ("name".to_string(), Json::Str("fly".to_string())),
                        ("version".to_string(), Json::Str("1.0.0".to_string())),
                    ]),
                ),
            ])),
            "initialized" => None,
            "shutdown" => {
                *self.shut_ok.lock().unwrap() = true;
                rpc(Json::Null)
            }
            "exit" => None,
            "textDocument/didOpen" => {
                self.did_open(params);
                None
            }
            "textDocument/didChange" => {
                self.did_change(params);
                None
            }
            "textDocument/didSave" => {
                self.did_save(params);
                None
            }
            "textDocument/didClose" => {
                self.did_close(params);
                None
            }
            "textDocument/hover" => rpc(self.hover(params)),
            "fly/forceCheck" => {
                let uri = params
                    .get("textDocument")
                    .and_then(|d| d.get("uri"))
                    .and_then(|u| u.as_str())
                    .unwrap_or("")
                    .to_string();
                self.check_uri(&uri);
                None
            }
            other => rpc_err(-32601, format!("方法未实现: {}", other)),
        }
    }

    fn did_open(&self, params: &Json) {
        let td = match params.get("textDocument") {
            Some(t) => t,
            None => return,
        };
        let Some(uri) = td.get("uri").and_then(|u| u.as_str()) else {
            return;
        };
        let text = td.get("text").and_then(|t| t.as_str()).unwrap_or("").to_string();
        self.docs.lock().unwrap().insert(
            uri.to_string(),
            Doc {
                text,
                version: 0,
            },
        );
        self.check_uri(uri);
    }

    fn did_change(self: &Arc<Server>, params: &Json) {
        let Some(uri) = params
            .get("textDocument")
            .and_then(|d| d.get("uri"))
            .and_then(|u| u.as_str())
        else {
            return;
        };
        let text = params
            .get("contentChanges")
            .and_then(|c| c.as_arr())
            .and_then(|arr| arr.last())
            .and_then(|c| c.get("text"))
            .and_then(|t| t.as_str())
            .unwrap_or("")
            .to_string();
        let mut docs = self.docs.lock().unwrap();
        let doc = docs
            .entry(uri.to_string())
            .or_insert_with(|| Doc { text: String::new(), version: 0 });
        doc.text = text;
        doc.version += 1;
        let version = doc.version;
        let uri = uri.to_string();
        drop(docs);
        // debounce：120ms 后检查，期间再次改动则跳过（版本号对比）。
        let this = Arc::clone(self);
        std::thread::spawn(move || {
            std::thread::sleep(std::time::Duration::from_millis(DEBOUNCE_MS));
            let docs = this.docs.lock().unwrap();
            if let Some(d) = docs.get(&uri) {
                if d.version == version {
                    drop(docs);
                    this.check_uri(&uri);
                }
            }
        });
    }

    fn did_save(&self, params: &Json) {
        let Some(uri) = params
            .get("textDocument")
            .and_then(|d| d.get("uri"))
            .and_then(|u| u.as_str())
        else {
            return;
        };
        self.check_uri(uri);
    }

    fn did_close(&self, params: &Json) {
        let Some(uri) = params
            .get("textDocument")
            .and_then(|d| d.get("uri"))
            .and_then(|u| u.as_str())
        else {
            return;
        };
        self.docs.lock().unwrap().remove(uri);
        self.publish(uri, &[]);
    }

    fn check_uri(&self, uri: &str) {
        let text = {
            let docs = self.docs.lock().unwrap();
            match docs.get(uri) {
                Some(d) => d.text.clone(),
                None => return,
            }
        };
        let path = uri_to_path(uri);
        let base = path.rsplit('/').next().unwrap_or(&path).to_string();
        let diags = match self.session() {
            Ok(mut g) => match g.as_mut().unwrap().check(&text, &path, false) {
                Ok(r) => {
                    if let Some(se) = r.server_error {
                        eprintln!("lsp: checkd 内部错误: {}", se);
                    }
                    r.diags
                }
                Err(e) => {
                    eprintln!("lsp: {}", e);
                    Vec::new()
                }
            },
            Err(e) => {
                eprintln!("lsp: {}", e);
                Vec::new()
            }
        };
        let mut list = Vec::new();
        for d in diags {
            let (mut line, col) = (d.pos.line, d.pos.col);
            if line == 0 {
                line = 1;
            }
            list.push(Json::Obj(vec![
                (
                    "range".to_string(),
                    Json::Obj(vec![
                        (
                            "start".to_string(),
                            Json::Obj(vec![
                                ("line".to_string(), Json::Num((line - 1) as f64)),
                                ("character".to_string(), Json::Num(col.saturating_sub(1) as f64)),
                            ]),
                        ),
                        (
                            "end".to_string(),
                            Json::Obj(vec![
                                ("line".to_string(), Json::Num((line - 1) as f64)),
                                ("character".to_string(), Json::Num(col as f64)),
                            ]),
                        ),
                    ]),
                ),
                ("severity".to_string(), Json::Num(1.0)),
                ("source".to_string(), Json::Str("fly".to_string())),
                ("code".to_string(), Json::Str(d.code.clone())),
                (
                    "message".to_string(),
                    Json::Str(format!("error[{}]: {}: {}", d.code, base, d.msg)),
                ),
            ]));
        }
        self.publish(uri, &list);
    }

    fn publish(&self, uri: &str, diags: &[Json]) {
        let _ = self.notify(
            "textDocument/publishDiagnostics",
            Json::Obj(vec![
                ("uri".to_string(), Json::Str(uri.to_string())),
                (
                    "diagnostics".to_string(),
                    Json::Arr(diags.to_vec()),
                ),
            ]),
        );
    }

    fn notify(&self, method: &str, params: Json) -> Result<(), String> {
        let msg = Json::Obj(vec![
            ("jsonrpc".to_string(), Json::Str("2.0".to_string())),
            ("method".to_string(), Json::Str(method.to_string())),
            ("params".to_string(), params),
        ]);
        let out = self.out.lock().unwrap();
        if let Some(tx) = out.as_ref() {
            tx.send(msg.encode().into_bytes()).map_err(|e| e.to_string())
        } else {
            Err("lsp 输出通道未初始化".to_string())
        }
    }

    fn hover(&self, params: &Json) -> Json {
        let Some(uri) = params
            .get("textDocument")
            .and_then(|d| d.get("uri"))
            .and_then(|u| u.as_str())
        else {
            return Json::Null;
        };
        let Some(line) = params
            .get("position")
            .and_then(|p| p.get("line"))
            .and_then(|l| l.as_num())
        else {
            return Json::Null;
        };
        let Some(char) = params
            .get("position")
            .and_then(|p| p.get("character"))
            .and_then(|c| c.as_num())
        else {
            return Json::Null;
        };
        let text = {
            let docs = self.docs.lock().unwrap();
            match docs.get(uri) {
                Some(d) => d.text.clone(),
                None => return Json::Null,
            }
        };
        let lines: Vec<&str> = text.split('\n').collect();
        let li = line as usize;
        if li >= lines.len() {
            return Json::Null;
        }
        let line_text = lines[li];
        let token = token_at(line_text, char as usize);
        if token.is_empty() {
            return Json::Null;
        }
        let mut parts = vec![format!("```fly\n{}\n```", line_text)];
        if let Some((_, doc)) = KEYWORD_DOCS.iter().find(|(k, _)| *k == token) {
            parts.push(doc.to_string());
        }
        Json::Obj(vec![(
            "contents".to_string(),
            Json::Obj(vec![
                ("kind".to_string(), Json::Str("markdown".to_string())),
                ("value".to_string(), Json::Str(parts.join("\n\n---\n\n"))),
            ]),
        )])
    }
}

fn token_at(line: &str, char: usize) -> String {
    let bytes = line.as_bytes();
    if char > bytes.len() {
        return String::new();
    }
    let mut start = char;
    while start > 0 && is_ident_char(bytes[start - 1]) {
        start -= 1;
    }
    let mut end = char;
    while end < bytes.len() && is_ident_char(bytes[end]) {
        end += 1;
    }
    line[start..end].to_string()
}

fn is_ident_char(c: u8) -> bool {
    c.is_ascii_alphanumeric() || c == b'_'
}

fn uri_to_path(uri: &str) -> String {
    if let Some(rest) = uri.strip_prefix("file://") {
        // 解码 %XX
        let mut out = Vec::new();
        let bytes = rest.as_bytes();
        let mut i = 0;
        while i < bytes.len() {
            if bytes[i] == b'%' && i + 2 < bytes.len() {
                if let Ok(b) = u8::from_str_radix(&rest[i + 1..i + 3], 16) {
                    out.push(b);
                    i += 3;
                    continue;
                }
            }
            out.push(bytes[i]);
            i += 1;
        }
        String::from_utf8_lossy(&out).to_string()
    } else {
        uri.to_string()
    }
}

fn read_message(r: &mut impl BufRead) -> Result<String, String> {
    let mut content_length: Option<usize> = None;
    loop {
        let mut line = String::new();
        let n = r
            .read_line(&mut line)
            .map_err(|e| format!("读取帧头失败: {}", e))?;
        if n == 0 {
            return Err("stdin 已关闭".to_string());
        }
        let line = line.trim();
        if line.is_empty() {
            break;
        }
        if let Some(rest) = line.strip_prefix("Content-Length:") {
            let n: usize = rest
                .trim()
                .parse()
                .map_err(|_| format!("非法 Content-Length {:?}", line))?;
            if n == 0 || n > 64 << 20 {
                return Err(format!("非法 Content-Length {:?}", line));
            }
            content_length = Some(n);
        }
    }
    let Some(n) = content_length else {
        return Err("缺少 Content-Length".to_string());
    };
    let mut body = vec![0u8; n];
    r.read_exact(&mut body)
        .map_err(|e| format!("读取消息体失败: {}", e))?;
    String::from_utf8(body).map_err(|_| "消息体非 UTF-8".to_string())
}

fn write_message(w: &mut impl Write, msg: &[u8]) -> Result<(), String> {
    write!(w, "Content-Length: {}\r\n\r\n", msg.len())
        .map_err(|e| format!("写入帧头失败: {}", e))?;
    w.write_all(msg)
        .map_err(|e| format!("写入消息体失败: {}", e))
}

// ---- 集成测试：spawn 真实 lsp 子进程，走完整 JSON-RPC 会话 ----
#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write as _;
    use std::process::{Command, Stdio};

    fn start_lsp() -> std::process::Child {
        let exe = std::env::current_exe().unwrap();
        let bin = exe
            .parent()
            .and_then(|d| d.parent())
            .map(|d| d.join("fly"))
            .filter(|p| p.exists())
            .unwrap_or_else(|| PathBuf::from("target/debug/fly"));
        Command::new(bin)
            .arg("lsp")
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::null())
            .spawn()
            .unwrap()
    }

    fn send(child: &mut std::process::Child, msg: &str) {
        let body = msg.as_bytes();
        let stdin = child.stdin.as_mut().unwrap();
        write!(stdin, "Content-Length: {}\r\n\r\n", body.len()).unwrap();
        stdin.write_all(body).unwrap();
        stdin.flush().unwrap();
    }

    fn recv(child: &mut std::process::Child) -> String {
        let mut stdout = child.stdout.as_mut().unwrap();
        let mut header = String::new();
        let mut buf = [0u8; 1];
        loop {
            stdout.read_exact(&mut buf).unwrap();
            header.push(buf[0] as char);
            if header.ends_with("\r\n\r\n") {
                break;
            }
        }
        let n: usize = header
            .split("Content-Length:")
            .nth(1)
            .unwrap()
            .trim()
            .parse()
            .unwrap();
        let mut body = vec![0u8; n];
        stdout.read_exact(&mut body).unwrap();
        String::from_utf8(body).unwrap()
    }

    #[test]
    fn full_session() {
        let mut child = start_lsp();
        send(
            &mut child,
            r#"{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}"#,
        );
        let r = recv(&mut child);
        assert!(r.contains("\"serverInfo\""));
        assert!(r.contains("\"hoverProvider\":true"));

        send(&mut child, r#"{"jsonrpc":"2.0","method":"initialized","params":{}}"#);
        let bad = "x = (1 +\n";
        let body = format!(
            r#"{{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{{"textDocument":{{"uri":"file:///t.fly","text":{}}}}}}}"#,
            serde_like_json_string(bad)
        );
        send(&mut child, &body);
        // 等待诊断发布
        let diag = recv(&mut child);
        assert!(diag.contains("publishDiagnostics"), "{}", diag);
        assert!(diag.contains("E0002"), "{}", diag);

        send(&mut child, r#"{"jsonrpc":"2.0","method":"textDocument/hover","params":{"textDocument":{"uri":"file:///t.fly"},"position":{"line":0,"character":0}}}"#);
        let h = recv(&mut child);
        assert!(h.contains("```fly"), "{}", h);

        send(&mut child, r#"{"jsonrpc":"2.0","id":2,"method":"shutdown","params":null}"#);
        let s = recv(&mut child);
        assert!(s.contains("\"result\":null"), "{}", s);
        send(&mut child, r#"{"jsonrpc":"2.0","method":"exit","params":null}"#);
        let status = child.wait().unwrap();
        assert!(status.success());
    }

    fn serde_like_json_string(s: &str) -> String {
        let mut out = String::new();
        for c in s.chars() {
            match c {
                '"' => out.push_str("\\\""),
                '\\' => out.push_str("\\\\"),
                '\n' => out.push_str("\\n"),
                c => out.push(c),
            }
        }
        format!("\"{}\"", out)
    }
}