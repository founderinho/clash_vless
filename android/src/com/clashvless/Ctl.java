package com.clashvless;

import android.content.Context;
import android.net.LocalSocket;
import android.net.LocalSocketAddress;

import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;

/** Client of the daemon's control.sock (same JSON-lines protocol the TUI uses). */
final class Ctl {
    interface Listener { void onUpdate(boolean stateChanged); }

    static volatile JSONObject state, status;
    static volatile boolean connected;
    static volatile Listener listener;
    private static OutputStream out;
    private static boolean started;

    /** Starts the (re)connect loop once; it also keeps an idle daemon up while the UI is visible. */
    static synchronized void start(Context c) {
        if (started) return;
        started = true;
        final Context app = c.getApplicationContext();
        new Thread(() -> {
            while (true) {
                try {
                    if (Core.uiVisible && !VpnSvc.running && !Core.daemonAlive()) Core.startDaemon(app, null);
                    session(app);
                } catch (Exception ignored) { }
                setOut(null);
                connected = false;
                fire(false);
                try { Thread.sleep(700); } catch (InterruptedException ignored) { }
            }
        }).start();
    }

    private static void session(Context c) throws Exception {
        try (LocalSocket s = new LocalSocket()) {
            s.connect(new LocalSocketAddress(Core.sock(c), LocalSocketAddress.Namespace.FILESYSTEM));
            setOut(s.getOutputStream());
            connected = true;
            BufferedReader r = new BufferedReader(new InputStreamReader(s.getInputStream(), StandardCharsets.UTF_8));
            for (String l; (l = r.readLine()) != null; ) {
                JSONObject e = new JSONObject(l);
                switch (e.optString("type")) {
                    case "state":
                        state = e.optJSONObject("state");
                        fire(true);
                        break;
                    case "status":
                        status = e.optJSONObject("status");
                        fire(false);
                        break;
                }
            }
        }
    }

    private static synchronized void setOut(OutputStream o) {
        out = o;
    }

    private static void fire(boolean stateChanged) {
        Listener l = listener;
        if (l != null) l.onUpdate(stateChanged);
    }

    static synchronized boolean send(JSONObject cmd) {
        try {
            if (out == null) throw new java.io.IOException("not connected");
            out.write((cmd.toString() + "\n").getBytes(StandardCharsets.UTF_8));
            out.flush();
            return true;
        } catch (Exception e) {
            Core.log("control: " + cmd.optString("cmd") + " dropped — " + e.getMessage());
            return false;
        }
    }

    static boolean cmd(String name, Object... kv) {
        try {
            JSONObject o = new JSONObject().put("cmd", name);
            for (int i = 0; i + 1 < kv.length; i += 2) o.put((String) kv[i], kv[i + 1]);
            return send(o);
        } catch (Exception e) {
            return false;
        }
    }

    /** Top-level state field replace — what the daemon's "patch" applies. */
    static boolean patch(String key, Object value) {
        try {
            return cmd("patch", "patch", new JSONObject().put(key, value));
        } catch (Exception e) {
            return false;
        }
    }
}
