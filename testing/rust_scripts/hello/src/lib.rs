use std::slice;
use std::str;

extern {
    fn println(ptr: *const u8, len: usize);
}

static HELLO: &'static str = "Hello, World!";

#[repr(C)]
pub struct Ptr {
    ptr: i32,
    len: i32,
}

#[no_mangle]
pub extern "C" fn hello(ptr: *const u8, len: usize, output: &mut (*const u8, usize) ) {
    let slice = unsafe { slice::from_raw_parts(ptr as _, len as _) };
    let string_from_host = str::from_utf8(&slice).unwrap();

    unsafe {
        let ptr_info = format!("input ptr {:?} {:?}", ptr, len);
        println(ptr_info.as_ptr(), ptr_info.len());
    }

    let formated = format!("Hello {}, ca marche pontiac", string_from_host);
    unsafe {
        println(formated.as_ptr(), formated.len());
    }

    unsafe {
        println(HELLO.as_ptr(), HELLO.len());
    }

    output.0 = formated.as_ptr();
    output.1 = formated.len();

}
