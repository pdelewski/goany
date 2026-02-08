class GoanyPanic {
    public static void goPanic(String msg) {
        System.err.println("panic: " + msg);
        System.exit(1);
    }
}
