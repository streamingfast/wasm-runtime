#[no_mangle]
pub extern "C" fn hello(name: String) -> String {
    return format!("Hello {}", name);
}
