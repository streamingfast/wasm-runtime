use std::slice;
extern {
    fn println(ptr: *const u8, len: usize);
}

#[no_mangle]
pub extern "C" fn read_big_bytes(ptr: *const u8, len: usize, output: &mut (*const u8, usize))  {
    unsafe {
        let ptr_info = format!("WTF");
        println(ptr_info.as_ptr(), ptr_info.len());
    }

    let slice = unsafe { slice::from_raw_parts(ptr as _, len as _) };
    unsafe {
        let ptr_info = format!("slice info {:?} ", slice.len());
        println(ptr_info.as_ptr(), ptr_info.len());
    }

    output.0 = slice.as_ptr();
    output.1 = slice.len();

}
