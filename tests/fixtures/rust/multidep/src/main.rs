use std::collections::HashMap;

mod dep1;

use dep1::get_example_text as example_text;

fn main() {
    let mut seen: HashMap<String, u32> = HashMap::new();
    seen.insert(example_text(), 1);
    println!("{}", seen.len());
}
