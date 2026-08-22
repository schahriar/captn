pub struct Widget {
    label: String,
}

impl Widget {
    pub fn describe(&self, prefix: &str) -> String {
        format!("{}{}", prefix, self.label)
    }
}
