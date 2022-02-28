extern {
    fn println(ptr: *const u8, len: usize);
}

#[no_mangle]
pub extern "C" fn read_big_bytes(ptr: *mut u8, len: usize, output: &mut (*const u8, usize))  {
    unsafe {
        let ptr_info = format!("WTF");
        println(ptr_info.as_ptr(), ptr_info.len());
    }

    unsafe {
        let mut input_data = Vec::from_raw_parts(ptr, len, len);
        let mut input_ptr = input_data.as_mut_ptr();
        input_ptr[1] = 2;
        let ptr_info = format!("slice info {:?} {:?} {:?}", input_ptr, input_data.len(), input_ptr[1]);
        println(ptr_info.as_ptr(), ptr_info.len());
        output.0 = input_ptr.as_ptr();
        output.1 = input_data.len();
        let done = format!("all done!");
        println(done.as_ptr(), done.len());
    }

    // slice[1] = 2;

}
