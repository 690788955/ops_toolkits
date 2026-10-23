namespace OpenOps.WinService {
    using System;
    using System.Runtime.InteropServices;
    using System.ComponentModel;

    public static class ServiceApi {
        [StructLayout(LayoutKind.Sequential)]
        private struct SERVICE_STATUS {
            public int dwServiceType;
            public int dwCurrentState;
            public int dwControlsAccepted;
            public int dwWin32ExitCode;
            public int dwServiceSpecificExitCode;
            public int dwCheckPoint;
            public int dwWaitHint;
        }

        [StructLayout(LayoutKind.Sequential)]
        private struct QUERY_SERVICE_STATUS_PROCESS {
            public SERVICE_STATUS ServiceStatus;
            public int dwProcessId;
            public int dwServiceFlags;
        }

        [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
        private static extern IntPtr OpenSCManager(string machineName, string databaseName, uint access);

        [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
        private static extern IntPtr OpenService(IntPtr manager, string name, uint access);

        [DllImport("advapi32.dll", SetLastError = true)]
        private static extern bool StartService(IntPtr service, int argc, string[] argv);

        [DllImport("advapi32.dll", SetLastError = true)]
        private static extern bool ControlService(IntPtr service, int control, ref SERVICE_STATUS status);

        [DllImport("advapi32.dll", SetLastError = true)]
        private static extern bool QueryServiceStatusEx(IntPtr service, int level, ref QUERY_SERVICE_STATUS_PROCESS status, int size, out int needed);

        [DllImport("advapi32.dll", SetLastError = true)]
        private static extern bool CloseServiceHandle(IntPtr handle);

        public static string State(string name) {
            IntPtr manager = OpenSCManager(null, null, 0x0001);
            if (manager == IntPtr.Zero) {
                throw new Win32Exception(Marshal.GetLastWin32Error(), "OpenSCManager failed");
            }
            try {
                IntPtr service = OpenService(manager, name, 0x0004);
                if (service == IntPtr.Zero) {
                    int code = Marshal.GetLastWin32Error();
                    if (code == 1060) {
                        throw new Win32Exception(code, "service not found: " + name);
                    }
                    throw new Win32Exception(code, "OpenService failed: " + name);
                }
                try {
                    int needed;
                    QUERY_SERVICE_STATUS_PROCESS info = new QUERY_SERVICE_STATUS_PROCESS();
                    if (!QueryServiceStatusEx(service, 0, ref info, Marshal.SizeOf(typeof(QUERY_SERVICE_STATUS_PROCESS)), out needed)) {
                        throw new Win32Exception(Marshal.GetLastWin32Error(), "QueryServiceStatusEx failed: " + name);
                    }
                    switch (info.ServiceStatus.dwCurrentState) {
                        case 1: return "stopped";
                        case 2: return "start-pending";
                        case 3: return "stop-pending";
                        case 4: return "running";
                        case 5: return "continue-pending";
                        case 6: return "pause-pending";
                        case 7: return "paused";
                        default: return "unknown";
                    }
                } finally {
                    CloseServiceHandle(service);
                }
            } finally {
                CloseServiceHandle(manager);
            }
        }

        public static void Stop(string name) {
            IntPtr manager = OpenSCManager(null, null, 0x0001);
            if (manager == IntPtr.Zero) {
                throw new Win32Exception(Marshal.GetLastWin32Error(), "OpenSCManager failed");
            }
            try {
                IntPtr service = OpenService(manager, name, 0x0020 | 0x0004);
                if (service == IntPtr.Zero) {
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "OpenService failed: " + name);
                }
                try {
                    SERVICE_STATUS status = new SERVICE_STATUS();
                    if (!ControlService(service, 0x0001, ref status)) {
                        int code = Marshal.GetLastWin32Error();
                        if (code != 1062) {
                            throw new Win32Exception(code, "ControlService failed: " + name);
                        }
                    }
                } finally {
                    CloseServiceHandle(service);
                }
            } finally {
                CloseServiceHandle(manager);
            }
        }

        public static void Start(string name) {
            IntPtr manager = OpenSCManager(null, null, 0x0001);
            if (manager == IntPtr.Zero) {
                throw new Win32Exception(Marshal.GetLastWin32Error(), "OpenSCManager failed");
            }
            try {
                IntPtr service = OpenService(manager, name, 0x0010 | 0x0004);
                if (service == IntPtr.Zero) {
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "OpenService failed: " + name);
                }
                try {
                    if (!StartService(service, 0, null)) {
                        int code = Marshal.GetLastWin32Error();
                        if (code != 1056) {
                            throw new Win32Exception(code, "StartService failed: " + name);
                        }
                    }
                } finally {
                    CloseServiceHandle(service);
                }
            } finally {
                CloseServiceHandle(manager);
            }
        }
    }
}
