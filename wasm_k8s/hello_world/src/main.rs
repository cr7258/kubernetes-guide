use std::time::Duration;
use std::thread::sleep;
fn main() {
    loop {
        println!("Hello, Rust");
        sleep(Duration::from_secs(5));
    }
}
