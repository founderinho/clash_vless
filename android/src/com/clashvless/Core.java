package com.clashvless;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.os.Build;

import java.io.BufferedReader;
import java.io.File;
import java.io.InputStreamReader;
import java.net.InetAddress;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.concurrent.TimeUnit;

/**
 * Owns the Go daemon process (lib/<abi>/libclashvless.so `run`) and the shared log.
 * The daemon runs whenever the UI is visible (idle: proxy + probes) or the VPN is on
 * (with the tun fd). It always owns state.json; the UI talks to it over control.sock.
 */
final class Core {
    interface Listener { void onLine(String line); }

    static final class Entry {
        final String line, level, module;

        Entry(String line) {
            this.line = line;
            String level = "INFO", module = "engine";
            String[] parts = line.split(" ", 4);
            if (parts.length == 4 && parts[0].length() == 12
                    && Arrays.asList("INFO", "WARN", "ERROR", "DEBUG").contains(parts[1])) {
                String[] body = parts[3].split(" ", 2);
                if (body.length == 2) {
                    level = parts[1];
                    module = body[0].split("/", 2)[0];
                }
            }
            this.level = level;
            this.module = module;
        }
    }

    private static final int LOG_MAX = 2000;
    private static final ArrayList<Entry> log = new ArrayList<>();
    private static Listener listener;

    static volatile boolean uiVisible;
    private static Process daemon;
    private static boolean daemonVpn;

    static synchronized void log(String line) {
        log.add(new Entry(line));
        if (log.size() > LOG_MAX) log.remove(0);
        if (listener != null) listener.onLine(line);
    }

    static synchronized void setListener(Listener l) {
        listener = l;
    }

    static synchronized void clearLog() {
        log.clear();
    }

    static synchronized List<String> modules() {
        Set<String> found = new LinkedHashSet<>();
        for (Entry entry : log) found.add(entry.module);
        List<String> result = new ArrayList<>(found);
        java.util.Collections.sort(result);
        return result;
    }

    static synchronized String filteredLog(Set<String> levels, String module, String query) {
        StringBuilder result = new StringBuilder();
        for (Entry entry : log) {
            if (!levels.contains(entry.level)) continue;
            if (!module.isEmpty() && !module.equals(entry.module)) continue;
            if (!textMatches(entry.line, query)) continue;
            result.append(entry.line).append('\n');
        }
        return result.toString();
    }

    private static boolean textMatches(String line, String query) {
        query = query.trim();
        if (query.isEmpty()) return true;
        String text = line.toLowerCase(Locale.ROOT);
        for (String alternative : query.split(" \\| ")) {
            boolean matches = true;
            int terms = 0;
            for (String term : alternative.split(" & ")) {
                term = term.trim().toLowerCase(Locale.ROOT);
                if (term.isEmpty()) continue;
                terms++;
                if (!text.contains(term)) { matches = false; break; }
            }
            if (matches && terms > 0) return true;
        }
        return false;
    }

    static String bin(Context c) {
        return c.getApplicationInfo().nativeLibraryDir + "/libclashvless.so";
    }

    static String dir(Context c) {
        return c.getFilesDir().getAbsolutePath();
    }

    /** Android 14+ ships updatable roots in the conscrypt APEX; Go only knows the /system dir. */
    static final String CERT_DIRS = "/apex/com.android.conscrypt/cacerts:/system/etc/security/cacerts";

    static String sock(Context c) {
        return dir(c) + "/control.sock";
    }

    /** files/cv: CLI as the app's user - `adb shell run-as com.clashvless sh files/cv status`. */
    static void writeCli(Context c, String dns) {
        String sh = "#!/system/bin/sh\nexport HOME=" + dir(c) + " CLASHVLESS_DNS=" + dns + " SSL_CERT_DIR=" + CERT_DIRS
                + "\nexec " + bin(c) + " --config " + dir(c) + " \"$@\"\n";
        try (java.io.FileWriter w = new java.io.FileWriter(new File(dir(c), "cv"))) {
            w.write(sh);
        } catch (java.io.IOException e) {
            log("write cv: " + e);
        }
        new File(dir(c), "cv").setExecutable(true);
    }

    static synchronized boolean daemonAlive() {
        return daemon != null;
    }

    /** (Re)starts the daemon; tunSock != null = VPN mode (it waits for the fd there). */
    static synchronized void startDaemon(Context c, String tunSock) throws java.io.IOException {
        stopDaemon();
        String dns = dns(c);
        writeCli(c, dns);
        List<String> cmd = new ArrayList<>(Arrays.asList(bin(c), "--config", dir(c), "run"));
        ProcessBuilder pb = new ProcessBuilder(cmd).redirectErrorStream(true).directory(new File(dir(c)));
        pb.environment().put("CLASHVLESS_DNS", dns);
        pb.environment().put("HOME", dir(c));
        pb.environment().put("SSL_CERT_DIR", CERT_DIRS);
        if (tunSock != null) pb.environment().put("CLASHVLESS_TUN_SOCK", tunSock);
        final Process p = pb.start();
        p.getOutputStream().close();
        daemon = p;
        daemonVpn = tunSock != null;
        log(daemonVpn ? "> daemon (VPN)" : "> daemon (idle - VPN off)");
        new Thread(() -> {
            int code = -1;
            try {
                pump(p);
                code = p.waitFor();
            } catch (Exception ignored) { }
            boolean wasVpn;
            synchronized (Core.class) {
                if (daemon != p) return; // stopped on purpose
                daemon = null;
                wasVpn = daemonVpn;
            }
            log("x daemon exited (" + code + ")");
            if (wasVpn && VpnSvc.instance != null) VpnSvc.instance.shutdown();
        }).start();
    }

    static synchronized void stopDaemon() {
        Process p = daemon;
        daemon = null;
        if (p == null) return;
        p.destroy(); // SIGTERM on API 26+ -> clean teardown; older Androids SIGKILL
        try {
            if (Build.VERSION.SDK_INT < 26) p.waitFor();
            else if (!p.waitFor(5, TimeUnit.SECONDS)) p.destroyForcibly();
        } catch (InterruptedException ignored) { }
    }

    private static void pump(Process p) throws java.io.IOException {
        try (BufferedReader r = new BufferedReader(new InputStreamReader(p.getInputStream()))) {
            for (String l; (l = r.readLine()) != null; ) {
                if (!l.startsWith("WARNING: linker:")) log(l); // old bionic nags about Go's DT_FLAGS_1
            }
        }
    }

    /** The underlying network's resolvers (Go has no /etc/resolv.conf to read on Android). */
    static String dns(Context c) {
        ConnectivityManager cm = c.getSystemService(ConnectivityManager.class);
        List<String> v4 = new ArrayList<>(), v6 = new ArrayList<>();
        for (Network n : cm.getAllNetworks()) {
            NetworkCapabilities nc = cm.getNetworkCapabilities(n);
            LinkProperties lp = cm.getLinkProperties(n);
            if (lp == null || nc == null || nc.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) continue;
            for (InetAddress a : lp.getDnsServers()) {
                String h = a.getHostAddress();
                if (h == null) continue;
                int pct = h.indexOf('%');
                if (pct >= 0) h = h.substring(0, pct);
                List<String> l = h.contains(":") ? v6 : v4;
                if (!l.contains(h)) l.add(h);
            }
        }
        v4.addAll(v6);
        return android.text.TextUtils.join(",", v4);
    }
}
