package com.clashvless;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.net.NetworkCapabilities;

import java.io.BufferedReader;
import java.io.File;
import java.io.InputStreamReader;
import java.net.InetAddress;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/** Runs the Go clashvless binary (shipped as lib/arm64-v8a/libclashvless.so) and holds the shared log. */
final class Core {
    interface Listener { void onLine(String line); }

    private static final int LOG_MAX = 64 * 1024;
    private static final StringBuilder log = new StringBuilder();
    private static Listener listener;

    static synchronized void log(String line) {
        log.append(line).append('\n');
        if (log.length() > LOG_MAX) log.delete(0, log.length() - LOG_MAX);
        if (listener != null) listener.onLine(line);
    }

    static synchronized String setListener(Listener l) {
        listener = l;
        return log.toString();
    }

    static String bin(Context c) {
        return c.getApplicationInfo().nativeLibraryDir + "/libclashvless.so";
    }

    static String dir(Context c) {
        return c.getFilesDir().getAbsolutePath();
    }

    /** files/cv: CLI as the app's user — `adb shell run-as com.clashvless sh files/cv status`. */
    static void writeCli(Context c, String dns) {
        String sh = "#!/system/bin/sh\nexport HOME=" + dir(c) + " CLASHVLESS_DNS=" + dns
                + "\nexec " + bin(c) + " --config " + dir(c) + " \"$@\"\n";
        try (java.io.FileWriter w = new java.io.FileWriter(new File(dir(c), "cv"))) {
            w.write(sh);
        } catch (java.io.IOException e) {
            log("write cv: " + e);
        }
        new File(dir(c), "cv").setExecutable(true);
    }

    static ProcessBuilder builder(Context c, List<String> args) {
        List<String> cmd = new ArrayList<>(Arrays.asList(bin(c), "--config", dir(c)));
        cmd.addAll(args);
        ProcessBuilder pb = new ProcessBuilder(cmd).redirectErrorStream(true).directory(new File(dir(c)));
        String dns = dns(c);
        writeCli(c, dns);
        pb.environment().put("CLASHVLESS_DNS", dns);
        pb.environment().put("HOME", dir(c));
        return pb;
    }

    /** Runs a one-shot CLI command, streaming its output into the log. Blocking. */
    static int exec(Context c, List<String> args) {
        log("$ clashvless " + android.text.TextUtils.join(" ", args));
        try {
            Process p = builder(c, args).start();
            p.getOutputStream().close();
            pump(p);
            return p.waitFor();
        } catch (Exception e) {
            log("error: " + e);
            return -1;
        }
    }

    static void pump(Process p) throws java.io.IOException {
        try (BufferedReader r = new BufferedReader(new InputStreamReader(p.getInputStream()))) {
            for (String l; (l = r.readLine()) != null; ) {
                if (!l.startsWith("WARNING: linker:")) log(l); // old bionic nags about Go's DT_FLAGS_1
            }
        }
    }

    /** The underlying network's resolvers (Go has no /etc/resolv.conf to read on Android). */
    static String dns(Context c) {
        ConnectivityManager cm = c.getSystemService(ConnectivityManager.class);
        StringBuilder sb = new StringBuilder();
        for (Network n : cm.getAllNetworks()) {
            NetworkCapabilities nc = cm.getNetworkCapabilities(n);
            LinkProperties lp = cm.getLinkProperties(n);
            if (lp == null || nc == null || nc.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) continue;
            for (InetAddress a : lp.getDnsServers()) {
                String h = a.getHostAddress();
                if (h == null || h.startsWith("198.18.")) continue;
                int pct = h.indexOf('%');
                if (pct >= 0) h = h.substring(0, pct);
                if (sb.length() > 0) sb.append(',');
                sb.append(h);
            }
        }
        return sb.toString();
    }
}
