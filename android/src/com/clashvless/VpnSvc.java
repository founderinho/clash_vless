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

/**
 * Owns the VPN: establishes the tun, restarts the daemon in VPN mode, and passes it
 * the tun fd over a unix socket. Our own UID is excluded from the VPN, so every
 * socket the daemon opens (exits, failover probes, sub fetches) goes direct.
 */
public class VpnSvc extends VpnService {
    static final String ACTION_START = "start", ACTION_STOP = "stop";
    static volatile boolean running;
    static volatile VpnSvc instance;

    private ParcelFileDescriptor tun;

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        instance = this;
        if (intent != null && ACTION_STOP.equals(intent.getAction())) {
            new Thread(this::shutdown).start();
            return START_NOT_STICKY;
        }
        foreground();
        if (!running) {
            running = true;
            Ctl.status = null;
            Ctl.start(this);
            Ctl.notificationListener = stateChanged -> updateNotification();
            Ctl.changed();
            VpnTileService.refresh(this);
            new Thread(this::startup).start();
        }
        return START_STICKY;
    }

    private void foreground() {
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationChannel channel = new NotificationChannel("vpn", "VPN connection", NotificationManager.IMPORTANCE_LOW);
            channel.setDescription("Current clashvless connection state and route");
            channel.setShowBadge(false);
            getSystemService(NotificationManager.class).createNotificationChannel(channel);
        }
        Notification notification = buildNotification();
        if (Build.VERSION.SDK_INT >= 34) startForeground(1, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);
        else startForeground(1, notification);
    }

    private Notification buildNotification() {
        Notification.Builder builder = Build.VERSION.SDK_INT >= 26
                ? new Notification.Builder(this, "vpn") : new Notification.Builder(this);
        org.json.JSONObject status = Ctl.status;
        boolean online = status != null && status.optInt("Tier") > 0;
        String details = status == null ? "Preparing the local engine and VPN interface"
                : online ? MainActivity.line(status) : "VPN is on | finding a working route";
        PendingIntent open = PendingIntent.getActivity(this, 0, new Intent(this, MainActivity.class),
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
        PendingIntent stop = PendingIntent.getService(this, 2,
                new Intent(this, VpnSvc.class).setAction(ACTION_STOP),
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
        return builder
                .setSmallIcon(R.drawable.clashvless_notification)
                .setColor(0xFF68D5BD)
                .setContentTitle(online ? "clashvless | Connected" : "clashvless | Connecting")
                .setContentText(details)
                .setStyle(new Notification.BigTextStyle().bigText(details))
                .setContentIntent(open)
                .addAction(0, "Disconnect", stop)
                .setCategory(Notification.CATEGORY_SERVICE)
                .setVisibility(Notification.VISIBILITY_PRIVATE)
                .setOnlyAlertOnce(true)
                .setOngoing(true)
                .build();
    }

    private void updateNotification() {
        getSystemService(NotificationManager.class).notify(1, buildNotification());
    }

    private synchronized void startup() {
        try {
            String sock = new File(getCacheDir(), "tun.sock").getAbsolutePath();
            Core.startDaemon(this, sock);
            for (int attempt = 0; Ctl.state == null && attempt < 50; attempt++) Thread.sleep(100);
            org.json.JSONObject st = Ctl.state;
            if (st == null) throw new IllegalStateException("Daemon configuration is unavailable");
            int mtu = st == null ? 0 : st.optInt("tun_mtu");
            if (mtu <= 0) mtu = 1500;
            String address = st == null ? "" : st.optString("tun_addr", "198.18.0.1/30");
            if (address.isEmpty()) address = "198.18.0.1/30";
            String[] cidr = address.split("/");
            String dns = st == null ? "" : st.optString("tun_static_dns", "8.8.8.8");
            if (dns.isEmpty()) dns = "8.8.8.8";
            tun = new Builder()
                    .setSession("clashvless")
                    .setMtu(mtu)
                    .addAddress(java.net.InetAddress.getByName(cidr[0]), cidr.length == 2 ? Integer.parseInt(cidr[1]) : 30)
                    .addRoute("0.0.0.0", 0)
                    .addDnsServer(dns)
                    .addDisallowedApplication(getPackageName())
                    .establish();
            if (tun == null) throw new IllegalStateException("VPN permission not granted");
            sendFd(sock, tun.getFileDescriptor());
            Core.log("> VPN up (tun fd handed to daemon)");
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

    synchronized void shutdown() {
        boolean was = running;
        running = false;
        Ctl.notificationListener = null;
        Ctl.changed();
        VpnTileService.refresh(this);
        if (was) Core.stopDaemon(); // the UI's connect loop brings an idle daemon back if visible
        if (tun != null) {
            try { tun.close(); } catch (Exception ignored) { }
            tun = null;
        }
        if (was) Core.log("x VPN off");
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
        instance = null;
        super.onDestroy();
    }
}
