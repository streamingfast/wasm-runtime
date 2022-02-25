use std::slice;
use std::str;

extern {
    fn println(ptr: *const u8, len: usize);
}

#[no_mangle]
pub extern "C" fn read_big_bytes(ptr: *const u8, len: usize) -> &'static [u8] {
    unsafe {
        slice::from_raw_parts(ptr as _, len as _);
        let ptr_info = format!("input ptr {:?} {:?}", ptr, len);
        println(ptr_info.as_ptr(), ptr_info.len());
    }

    &[0]
}
