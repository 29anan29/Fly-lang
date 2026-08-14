// version.rs：版本信息结构（checkd 握手协议携带）。
pub struct Version {
    pub version: &'static str,
    pub commit: &'static str,
    pub repo: &'static str,
}

pub const CURRENT: Version = Version {
    version: env!("FLY_VERSION"),
    commit: env!("FLY_COMMIT"),
    repo: env!("FLY_REPO"),
};

fn is_dev() -> bool {
    let v = CURRENT.version.to_lowercase();
    v.is_empty() || v == "dev" || v.contains("-dev")
}

pub fn string() -> String {
    if is_dev() {
        if CURRENT.commit.len() >= 7 {
            return format!("{} ({})", CURRENT.version, &CURRENT.commit[..7]);
        }
        return CURRENT.version.to_string();
    }
    format!("{} (release)", CURRENT.version)
}

pub fn trim_tag(tag: &str) -> &str {
    if tag.len() > 1 && (tag.starts_with('v') || tag.starts_with('V')) {
        return &tag[1..];
    }
    tag
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn trim_tag_strips_v() {
        assert_eq!(trim_tag("v1.2.3"), "1.2.3");
        assert_eq!(trim_tag("1.2.3"), "1.2.3");
    }
}
