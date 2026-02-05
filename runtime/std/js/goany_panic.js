function goany_panic(msg) {
    console.error("panic: " + msg);
    if (typeof process !== 'undefined') {
        process.exit(1);
    } else {
        throw new Error("panic: " + msg);
    }
}
