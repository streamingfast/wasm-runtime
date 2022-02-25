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
pub extern "C" fn hello(ptr: *const u8, len: usize, output: &mut (*const u8, usize), output2: &mut (*const u8, usize) ) -> i32 {
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

    let from_within = format!("This {}, comes from within", string_from_host);
    
    output.0 = from_within.as_ptr();
    output.1 = from_within.len();

    output2.0 = string_from_host.as_ptr();
    output2.1 = string_from_host.len();

    42
}
