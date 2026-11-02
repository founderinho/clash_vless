package com.clashvless;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.net.LocalSocket;
import android.net.LocalSocketAddress;
import android.net.VpnService;
import android.os.Build;
import android.os.ParcelFileDescriptor;

import java.io.File;
import java.io.FileDescriptor;
import java.io.OutputStream;
import java.util.Collections;
import java.util.concurrent.TimeUnit;

/**
 * Owns the VPN: establishes the tun, spawns the Go daemon (`run`), and passes it
 * the tun fd over a unix socket. Our own UID is excluded from the VPN, so every
 * socket the daemon opens (exits, failover probes, sub fetches) goes direct.
 */
public class VpnSvc extends VpnService {
    static final String ACTION_START = "start", ACTION_STOP = "stop";
    static volatile boolean running;

    private ParcelFileDescriptor tun;
    private Process daemon;

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && ACTION_STOP.equals(intent.getAction())) {
            new Thread(this::shutdown).start();
            return START_NOT_STICKY;
        }
        foreground();
        if (!running) {
            running = true;
            new Thread(this::startup).start();
        }
        return START_STICKY;
    }

    private void foreground() {
        Notification.Builder nb;
        if (Build.VERSION.SDK_INT >= 26) {
            getSystemService(NotificationManager.class)
                    .createNotificationChannel(new NotificationChannel("vpn", "VPN", NotificationManager.IMPORTANCE_LOW));
            nb = new Notification.Builder(this, "vpn");
        } else {
            nb = new Notification.Builder(this);
        }
        PendingIntent pi = PendingIntent.getActivity(this, 0, new Intent(this, MainActivity.class), PendingIntent.FLAG_IMMUTABLE);
        Notification n = nb
                .setSmallIcon(android.R.drawable.ic_lock_lock)
                .setContentTitle("clashvless")
                .setContentText("VPN on")
                .setContentIntent(pi)
                .setOngoing(true)
                .build();
        if (Build.VERSION.SDK_INT >= 34) startForeground(1, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);
        else startForeground(1, n);
    }

    private synchronized void startup() {
        try {
            String sock = new File(getCacheDir(), "tun.sock").getAbsolutePath();
            ProcessBuilder pb = Core.builder(this, Collections.singletonList("run"));
            pb.environment().put("CLASHVLESS_TUN_SOCK", sock);
            daemon = pb.start();
            daemon.getOutputStream().close();
            final Process p = daemon;
            new Thread(() -> {
                int code = -1;
                try { Core.pump(p); code = p.waitFor(); } catch (Exception ignored) { }
                Core.log("■ daemon exited (" + code + ")");
                if (p == daemon) shutdown();
            }).start();

            tun = new Builder()
                    .setSession("clashvless")
                    .setMtu(1500)
                    .addAddress("198.18.0.1", 30)
                    .addRoute("0.0.0.0", 0)
                    .addDnsServer("8.8.8.8")
                    .addDisallowedApplication(getPackageName())
                    .establish();
            if (tun == null) throw new IllegalStateException("VPN permission not granted");
            sendFd(sock, tun.getFileDescriptor());
            Core.log("▶ VPN up (tun fd handed to daemon)");
        } catch (Exception e) {
            Core.log("VPN start failed: " + e);
            shutdown();
        }
    }

    private static void sendFd(String path, FileDescriptor fd) throws Exception {
        Exception last = null;
        for (int i = 0; i < 50; i++) {
            try (LocalSocket s = new LocalSocket()) {
                s.connect(new LocalSocketAddress(path, LocalSocketAddress.Namespace.FILESYSTEM));
                s.setFileDescriptorsForSend(new FileDescriptor[]{fd});
                OutputStream o = s.getOutputStream();
                o.write(1);
                o.flush();
                Thread.sleep(200); // let the daemon recvmsg before the socket closes
                return;
            } catch (Exception e) {
                last = e;
                Thread.sleep(200);
            }
        }
        throw last;
    }

    private synchronized void shutdown() {
        Process p = daemon;
        daemon = null;
        if (p != null) {
            p.destroy(); // SIGTERM on API 26+ → daemon tears down cleanly; older Androids SIGKILL
            try {
                if (Build.VERSION.SDK_INT < 26) p.waitFor();
                else if (!p.waitFor(5, TimeUnit.SECONDS)) p.destroyForcibly();
            } catch (InterruptedException ignored) { }
        }
        if (tun != null) {
            try { tun.close(); } catch (Exception ignored) { }
            tun = null;
        }
        if (running) Core.log("■ VPN off");
        running = false;
        stopForeground(STOP_FOREGROUND_REMOVE);
        stopSelf();
    }

    @Override
    public void onRevoke() {
        new Thread(this::shutdown).start();
    }

    @Override
    public void onDestroy() {
        if (running) shutdown();
        super.onDestroy();
    }
}
