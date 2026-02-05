public static class GoanyRuntime
{
    public static void goany_panic(string msg)
    {
        Console.Error.WriteLine("panic: " + msg);
        Environment.Exit(1);
    }
}
