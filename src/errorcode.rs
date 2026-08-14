// errorcode.rs：错误码全量抽查（errorinfo.rs 生成的 ERROR_INFO 与 error_code 表一致性）。
use crate::errorinfo::{ErrorInfo, ERROR_INFO};

pub fn info_for_code(code: &str) -> Option<&'static ErrorInfo> {
    ERROR_INFO.iter().find(|i| i.code == code)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn registry_complete() {
        assert_eq!(ERROR_INFO.len(), 66, "错误码数量应与 Go 版一致");
    }

    #[test]
    fn codes_sequential_from_e0001() {
        for (i, info) in ERROR_INFO.iter().enumerate() {
            let want = format!("E{:04}", i + 1);
            assert_eq!(info.code, want, "错误码应连续编号 E0001 起");
        }
    }

    #[test]
    fn example_block_format() {
        let info = info_for_code("E0031").expect("E0031 应存在");
        assert!(info.example.starts_with("error[E0031]: "));
        assert!(info.example.contains("  --> example.fly:"));
        assert!(info.example.contains("   = help: "));
    }
}