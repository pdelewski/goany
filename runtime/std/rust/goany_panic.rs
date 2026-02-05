pub fn goany_panic(msg: String) {
    eprintln!("panic: {}", msg);
    std::process::exit(1);
}
