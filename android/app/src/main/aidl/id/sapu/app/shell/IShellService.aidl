package id.sapu.app.shell;

interface IShellService {
    // Runs a shell command as the Shizuku user (shell, uid 2000) and returns stdout+stderr.
    String run(String command, int timeoutSeconds) = 1;
    void destroy() = 16777114; // Shizuku UserService convention
}
