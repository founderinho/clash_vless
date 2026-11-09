package com.clashvless;

import android.app.PendingIntent;
import android.content.ComponentName;
import android.content.Intent;
import android.graphics.drawable.Icon;
import android.net.VpnService;
import android.os.Build;
import android.service.quicksettings.Tile;
import android.service.quicksettings.TileService;

import org.json.JSONObject;

public class VpnTileService extends TileService {
    @Override
    public void onStartListening() {
        super.onStartListening();
        Ctl.start(this);
        Ctl.tileListener = stateChanged -> updateTile();
        updateTile();
    }

    @Override
    public void onStopListening() {
        Ctl.tileListener = null;
        super.onStopListening();
    }

    @Override
    public void onClick() {
        super.onClick();
        if (VpnSvc.running) {
            startService(new Intent(this, VpnSvc.class).setAction(VpnSvc.ACTION_STOP));
            return;
        }
        if (VpnService.prepare(this) != null) {
            Intent open = new Intent(this, MainActivity.class)
                    .setAction(MainActivity.ACTION_CONNECT)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_SINGLE_TOP);
            if (Build.VERSION.SDK_INT >= 34) {
                startActivityAndCollapse(PendingIntent.getActivity(this, 3, open,
                        PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT));
            } else {
                startActivityAndCollapse(open);
            }
            return;
        }
        Intent start = new Intent(this, VpnSvc.class).setAction(VpnSvc.ACTION_START);
        if (Build.VERSION.SDK_INT >= 26) startForegroundService(start);
        else startService(start);
    }

    private void updateTile() {
        Tile tile = getQsTile();
        if (tile == null) return;
        tile.setIcon(Icon.createWithResource(this, R.drawable.clashvless_status));
        tile.setLabel("clashvless");
        tile.setState(VpnSvc.running ? Tile.STATE_ACTIVE : Tile.STATE_INACTIVE);
        if (Build.VERSION.SDK_INT >= 29) {
            JSONObject status = Ctl.status;
            boolean online = VpnSvc.running && status != null && status.optInt("Tier") > 0;
            tile.setSubtitle(!VpnSvc.running ? "Off" : online ? "Connected" : "Connecting");
            tile.setContentDescription("clashvless VPN, " + (!VpnSvc.running ? "off" : online ? "connected" : "connecting"));
        }
        tile.updateTile();
    }

    static void refresh(android.content.Context context) {
        TileService.requestListeningState(context, new ComponentName(context, VpnTileService.class));
    }
}
