// json.rs：极简 JSON 解析/序列化（LSP 用，std-only，不处理数字科学计数等边缘）。
use std::fmt::Write as _;

#[derive(Debug, Clone, PartialEq)]
pub enum Json {
    Null,
    Bool(bool),
    Num(f64),
    Str(String),
    Arr(Vec<Json>),
    Obj(Vec<(String, Json)>),
}

impl Json {
    pub fn parse(s: &str) -> Result<Json, String> {
        let mut p = Parser { s, i: 0 };
        let v = p.value()?;
        p.skip_ws();
        if p.i != s.len() {
            return Err("JSON 尾部有多余内容".to_string());
        }
        Ok(v)
    }

    pub fn get(&self, key: &str) -> Option<&Json> {
        match self {
            Json::Obj(kv) => kv.iter().find(|(k, _)| k == key).map(|(_, v)| v),
            _ => None,
        }
    }

    pub fn as_str(&self) -> Option<&str> {
        match self {
            Json::Str(s) => Some(s),
            _ => None,
        }
    }

    pub fn as_num(&self) -> Option<f64> {
        match self {
            Json::Num(n) => Some(*n),
            _ => None,
        }
    }

    pub fn as_arr(&self) -> Option<&[Json]> {
        match self {
            Json::Arr(a) => Some(a),
            _ => None,
        }
    }

    pub fn is_null(&self) -> bool {
        matches!(self, Json::Null)
    }

    // 序列化（LSP 输出场景，键保持插入顺序）。
    pub fn encode(&self) -> String {
        let mut out = String::new();
        self.write_to(&mut out);
        out
    }

    fn write_to(&self, out: &mut String) {
        match self {
            Json::Null => out.push_str("null"),
            Json::Bool(b) => out.push_str(if *b { "true" } else { "false" }),
            Json::Num(n) => {
                if n.fract() == 0.0 && n.is_finite() {
                    let _ = write!(out, "{}", *n as i64);
                } else {
                    let _ = write!(out, "{}", n);
                }
            }
            Json::Str(s) => write_json_string(out, s),
            Json::Arr(a) => {
                out.push('[');
                for (i, v) in a.iter().enumerate() {
                    if i > 0 {
                        out.push(',');
                    }
                    v.write_to(out);
                }
                out.push(']');
            }
            Json::Obj(kv) => {
                out.push('{');
                for (i, (k, v)) in kv.iter().enumerate() {
                    if i > 0 {
                        out.push(',');
                    }
                    write_json_string(out, k);
                    out.push(':');
                    v.write_to(out);
                }
                out.push('}');
            }
        }
    }
}

fn write_json_string(out: &mut String, s: &str) {
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => {
                let _ = write!(out, "\\u{:04x}", c as u32);
            }
            c => out.push(c),
        }
    }
    out.push('"');
}

struct Parser<'a> {
    s: &'a str,
    i: usize,
}

impl<'a> Parser<'a> {
    fn skip_ws(&mut self) {
        while self.i < self.s.len() {
            let c = self.s.as_bytes()[self.i];
            if c == b' ' || c == b'\t' || c == b'\n' || c == b'\r' {
                self.i += 1;
            } else {
                break;
            }
        }
    }

    fn peek(&mut self) -> Option<char> {
        self.skip_ws();
        self.s[self.i..].chars().next()
    }

    fn value(&mut self) -> Result<Json, String> {
        match self.peek() {
            None => Err("JSON 为空".to_string()),
            Some('{') => self.object(),
            Some('[') => self.array(),
            Some('"') => Ok(Json::Str(self.string()?)),
            Some('t') => {
                self.lit("true")?;
                Ok(Json::Bool(true))
            }
            Some('f') => {
                self.lit("false")?;
                Ok(Json::Bool(false))
            }
            Some('n') => {
                self.lit("null")?;
                Ok(Json::Null)
            }
            Some(c) if c == '-' || c.is_ascii_digit() => self.number(),
            Some(c) => Err(format!("JSON 非法字符 {:?}", c)),
        }
    }

    fn lit(&mut self, s: &str) -> Result<(), String> {
        if self.s[self.i..].starts_with(s) {
            self.i += s.len();
            Ok(())
        } else {
            Err(format!("JSON 字面量 {} 不匹配", s))
        }
    }

    fn number(&mut self) -> Result<Json, String> {
        let start = self.i;
        while self.i < self.s.len() {
            let c = self.s.as_bytes()[self.i];
            if c.is_ascii_digit() || c == b'-' || c == b'+' || c == b'.' || c == b'e' || c == b'E' {
                self.i += 1;
            } else {
                break;
            }
        }
        self.s[start..self.i]
            .parse::<f64>()
            .map(Json::Num)
            .map_err(|e| format!("JSON 数字非法: {}", e))
    }

    fn string(&mut self) -> Result<String, String> {
        self.i += 1;
        let mut out = String::new();
        loop {
            if self.i >= self.s.len() {
                return Err("JSON 字符串未闭合".to_string());
            }
            let c = self.s[self.i..].chars().next().unwrap();
            self.i += c.len_utf8();
            match c {
                '"' => return Ok(out),
                '\\' => {
                    let e = self.s[self.i..].chars().next().ok_or("JSON 转义截断")?;
                    self.i += e.len_utf8();
                    match e {
                        '"' => out.push('"'),
                        '\\' => out.push('\\'),
                        '/' => out.push('/'),
                        'b' => out.push('\u{0008}'),
                        'f' => out.push('\u{000C}'),
                        'n' => out.push('\n'),
                        'r' => out.push('\r'),
                        't' => out.push('\t'),
                        'u' => {
                            let hex: String = self.s[self.i..].chars().take(4).collect();
                            if hex.len() != 4 {
                                return Err("JSON \\u 转义截断".to_string());
                            }
                            self.i += 4;
                            let cp = u32::from_str_radix(&hex, 16)
                                .map_err(|_| "JSON \\u 非法".to_string())?;
                            out.push(char::from_u32(cp).ok_or("JSON \\u 码点非法")?);
                        }
                        other => return Err(format!("JSON 非法转义 \\{}", other)),
                    }
                }
                other => out.push(other),
            }
        }
    }

    fn array(&mut self) -> Result<Json, String> {
        self.i += 1;
        let mut arr = Vec::new();
        loop {
            match self.peek() {
                None => return Err("JSON 数组未闭合".to_string()),
                Some(']') => {
                    self.i += 1;
                    return Ok(Json::Arr(arr));
                }
                Some(_) => arr.push(self.value()?),
            }
            self.skip_ws();
            if self.i < self.s.len() {
                match self.s.as_bytes()[self.i] {
                    b',' => {
                        self.i += 1;
                    }
                    b']' => {
                        self.i += 1;
                        return Ok(Json::Arr(arr));
                    }
                    _ => return Err("JSON 数组缺少逗号".to_string()),
                }
            }
        }
    }

    fn object(&mut self) -> Result<Json, String> {
        self.i += 1;
        let mut kv = Vec::new();
        loop {
            match self.peek() {
                None => return Err("JSON 对象未闭合".to_string()),
                Some('}') => {
                    self.i += 1;
                    return Ok(Json::Obj(kv));
                }
                Some('"') => {
                    let key = self.string()?;
                    self.skip_ws();
                    if self.i >= self.s.len() || self.s.as_bytes()[self.i] != b':' {
                        return Err("JSON 对象缺少冒号".to_string());
                    }
                    self.i += 1;
                    let v = self.value()?;
                    kv.push((key, v));
                }
                Some(c) => return Err(format!("JSON 对象键非法 {:?}", c)),
            }
            self.skip_ws();
            if self.i < self.s.len() {
                match self.s.as_bytes()[self.i] {
                    b',' => {
                        self.i += 1;
                    }
                    b'}' => {
                        self.i += 1;
                        return Ok(Json::Obj(kv));
                    }
                    _ => return Err("JSON 对象缺少逗号".to_string()),
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_basic() {
        let v = Json::parse(r#"{"a":1,"b":[true,false,null],"c":"x\n\"y"}"#).unwrap();
        assert_eq!(v.get("a").unwrap().as_num().unwrap(), 1.0);
        let arr = v.get("b").unwrap().as_arr().unwrap();
        assert_eq!(arr.len(), 3);
        assert_eq!(v.get("c").unwrap().as_str().unwrap(), "x\n\"y");
    }

    #[test]
    fn encode_roundtrip() {
        let v = Json::parse(r#"{"uri":"file:///a.fly","diags":[{"severity":1}]}"#).unwrap();
        let s = v.encode();
        let v2 = Json::parse(&s).unwrap();
        assert_eq!(v, v2);
    }

    #[test]
    fn encode_unicode() {
        let j = Json::Obj(vec![("msg".to_string(), Json::Str("你好".to_string()))]);
        let s = j.encode();
        assert!(s.contains("你好"));
    }
}