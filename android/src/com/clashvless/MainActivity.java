package com.clashvless;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Intent;
import android.content.res.ColorStateList;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.BitmapDrawable;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.RippleDrawable;
import android.net.VpnService;
import android.os.Build;
import android.os.Bundle;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.GridLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.Switch;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;
import java.util.function.Consumer;
import java.util.function.IntConsumer;

/** Native Android dashboard over the same daemon protocol as desktop. */
public class MainActivity extends Activity {
    static final String ACTION_CONNECT = "com.clashvless.CONNECT";
    private static final String[] TABS = {"Settings", "Overview", "Log"};
    private static final String[] ICONS = {"settings", "overview", "activity"};
    private static final int CONFIG = 0, STATUS = 1, LOG = 2;
    private static int BG = Color.rgb(15, 19, 25), CARD = Color.rgb(24, 30, 38);
    private static int TEXT = Color.rgb(230, 235, 241), MUTED = Color.rgb(145, 157, 171);
    private static int ACCENT = Color.rgb(104, 213, 189), DANGER = Color.rgb(244, 126, 139);

    private int tab = STATUS, configSection;
    private boolean switchUpdating;
    private TextView connectionLabel;
    private Switch vpnSwitch;
    private final Button[] tabBtns = new Button[TABS.length];
    private FrameLayout contentFrame;
    private LinearLayout body;
    private ScrollView scroll;
    private TextView logView;

    @Override
    protected void onCreate(Bundle saved) {
        super.onCreate(saved);
        loadBranding();
        if (Build.VERSION.SDK_INT >= 33) requestPermissions(new String[]{"android.permission.POST_NOTIFICATIONS"}, 0);
        getWindow().setStatusBarColor(BG);
        getWindow().setNavigationBarColor(BG);

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setBackgroundColor(BG);
        root.setPadding(dp(14), dp(8), dp(14), 0);

        LinearLayout top = new LinearLayout(this);
        top.setGravity(Gravity.CENTER_VERTICAL);
        ImageView logo = new ImageView(this);
        logo.setImageBitmap(loadBitmap("logo"));
        top.addView(logo, new LinearLayout.LayoutParams(dp(38), dp(38)));
        TextView headline = text("clashvless", 21, TEXT, true);
        headline.setPadding(dp(10), 0, dp(10), 0);
        top.addView(headline, new LinearLayout.LayoutParams(0, -2, 1));
        LinearLayout connection = new LinearLayout(this);
        connection.setOrientation(LinearLayout.VERTICAL);
        connection.setGravity(Gravity.CENTER_HORIZONTAL);
        connection.setMinimumWidth(dp(88));
        connectionLabel = text("Disconnected", 12, MUTED, true);
        connectionLabel.setSingleLine(true);
        connectionLabel.setGravity(Gravity.CENTER);
        connection.addView(connectionLabel, new LinearLayout.LayoutParams(dp(88), -2));
        vpnSwitch = new Switch(this);
        vpnSwitch.setShowText(false);
        vpnSwitch.setThumbTintList(new ColorStateList(new int[][]{{android.R.attr.state_checked}, {}}, new int[]{BG, MUTED}));
        vpnSwitch.setTrackTintList(new ColorStateList(new int[][]{{android.R.attr.state_checked}, {}}, new int[]{ACCENT, Color.rgb(51, 66, 88)}));
        vpnSwitch.setOnCheckedChangeListener((view, on) -> {
            if (!switchUpdating) toggleVpn();
        });
        connection.addView(vpnSwitch, new LinearLayout.LayoutParams(dp(78), dp(44)));
        top.addView(connection);
        root.addView(top);

        contentFrame = new FrameLayout(this);
        scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        body = new LinearLayout(this);
        body.setOrientation(LinearLayout.VERTICAL);
        body.setPadding(0, dp(6), 0, dp(20));
        scroll.addView(body, new ScrollView.LayoutParams(-1, -2));
        contentFrame.addView(scroll, new FrameLayout.LayoutParams(-1, -1));
        root.addView(contentFrame, new LinearLayout.LayoutParams(-1, 0, 1));

        LinearLayout tabs = new LinearLayout(this);
        tabs.setGravity(Gravity.CENTER);
        tabs.setPadding(0, dp(3), 0, dp(5));
        for (int i = 0; i < TABS.length; i++) {
            final int selected = i;
            tabBtns[i] = btn(TABS[i], false, () -> navigate(selected));
            tabBtns[i].setTextSize(11);
            setFooterIcon(tabBtns[i], ICONS[i], MUTED);
            tabs.addView(tabBtns[i], new LinearLayout.LayoutParams(0, dp(56), 1));
        }
        root.addView(tabs);
        setContentView(root);
        Ctl.start(this);
        handleConnectIntent(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        handleConnectIntent(intent);
    }

    private void handleConnectIntent(Intent intent) {
        if (intent != null && ACTION_CONNECT.equals(intent.getAction()) && !VpnSvc.running) {
            contentFrame.post(this::toggleVpn);
            intent.setAction(null);
        }
    }

    @Override
    protected void onStart() {
        super.onStart();
        Core.uiVisible = true;
        Ctl.listener = stateChanged -> runOnUiThread(() -> {
            if (tab == STATUS || (stateChanged && tab == CONFIG)) render();
            else renderHead();
        });
        render();
    }

    @Override
    protected void onStop() {
        Core.uiVisible = false;
        Ctl.listener = null;
        Core.setListener(null);
        if (!VpnSvc.running && !isChangingConfigurations()) new Thread(Core::stopDaemon).start();
        super.onStop();
    }

    private void navigate(int selected) {
        tab = selected;
        render();
    }

    private void render() {
        renderHead();
        for (int i = 0; i < TABS.length; i++) styleTab(tabBtns[i], i == tab);
        body.removeAllViews();
        if (tab != LOG) Core.setListener(null);
        JSONObject st = Ctl.state;
        if (tab != LOG && (!Ctl.connected || st == null)) {
            body.addView(card("Connecting", "Starting the local engine and reading its state..."));
            return;
        }
        if (tab == STATUS) renderStatus(st, body);
        else if (tab == LOG) renderLog();
        else renderConfig(st);
    }

    private void renderHead() {
        JSONObject s = Ctl.status;
        boolean online = VpnSvc.running && s != null && s.optInt("Tier") > 0;
        connectionLabel.setText(!VpnSvc.running ? "Disconnected" : online ? "Connected" : "Connecting");
        connectionLabel.setTextColor(online ? ACCENT : MUTED);
        switchUpdating = true;
        vpnSwitch.setChecked(VpnSvc.running);
        switchUpdating = false;
    }

    static String line(JSONObject s) {
        int tier = s.optInt("Tier");
        long ms = s.optLong("Egress") / 1_000_000;
        String main = s.optString("Main");
        if (main.isEmpty()) main = "main";
        if (tier == 0) return s.optString("Err", "Waiting for a route");
        if (tier == 1) return "T1 direct -> " + main + " | " + ms + " ms";
        return "T" + tier + " " + s.optString("Entry") + " -> " + main + " | " + ms + " ms";
    }

    private void renderStatus(JSONObject st, LinearLayout target) {
        JSONObject s = Ctl.status;
        boolean online = s != null && s.optInt("Tier") > 0;
        LinearLayout hero = card(online ? "Connected" : "Not connected",
                online ? "Your private route is ready" : "Waiting for a working exit");
        hero.addView(text(s == null ? "" : line(s), 14, online ? ACCENT : MUTED, false));
        hero.addView(spacer(10));
        LinearLayout metrics = new LinearLayout(this);
        addMetric(metrics, "Latency", online ? s.optLong("Egress") / 1_000_000 + " ms" : "-");
        addMetric(metrics, "Healthy nodes", okCount(s == null ? null : s.optJSONArray("Country")) + " / " + okCount(s == null ? null : s.optJSONArray("Bypass")));
        addMetric(metrics, "Active tier", online ? "T" + s.optInt("Tier") : "-");
        hero.addView(metrics);
        target.addView(hero);
        if (s != null && !s.optString("TunErr").isEmpty()) target.addView(card("VPN error", s.optString("TunErr")));

        LinearLayout endpoints = card("Local endpoints", VpnSvc.running ? "System-wide VPN is on" : "Native SOCKS5 listeners");
        JSONArray slots = st.optJSONArray("slots");
        for (int i = 0; slots != null && i < slots.length(); i++) {
            JSONObject sl = slots.optJSONObject(i);
            if (!sl.optBoolean("enabled")) continue;
            String address = "socks5://" + listenHost(st) + ":" + sl.optInt("port");
            LinearLayout row = row(text(sl.optString("name") + "\n" + address, 13, TEXT, false),
                    btn("Copy", false, () -> copy(address)));
            endpoints.addView(row);
        }
        target.addView(endpoints);
        if (s != null) {
            LinearLayout pools = card("Entry pools", "Live health and tier selection");
            pools.addView(text("Country | T2     " + okCount(s.optJSONArray("Country")) + " online", 14, TEXT, false));
            pools.addView(text("Bypass | T3       " + okCount(s.optJSONArray("Bypass")) + " online", 14, TEXT, false));
            if (!s.optString("Note").isEmpty()) pools.addView(text(s.optString("Note"), 12, MUTED, false));
            target.addView(pools);
        }
        if (!anyMain(st)) {
            LinearLayout setup = card("Finish your setup", "Subscriptions provide hops. Add a final VLESS exit to carry your traffic.");
            setup.addView(btn("Add final exit", true, () -> askMain(st, 0)));
            target.addView(setup);
        }
        target.addView(btn("Recheck every route", false, () -> Ctl.cmd("kick")));
    }

    private void renderSubs(JSONObject st) {
        LinearLayout controls = card("Providers", "Fetch entry nodes and choose how subscriptions are combined");
        controls.addView(row(btn("Add subscription", true, this::addSub), btn("Refresh all", false, () -> {
            if (Ctl.cmd("refetch")) fetching("refetching all subscriptions");
        })));
        controls.addView(toggle("Aggregate all subscriptions", st.optBoolean("auto_select"), on -> Ctl.patch("auto_select", on)));
        body.addView(controls);
        JSONArray subs = st.optJSONArray("subs");
        if (subs == null || subs.length() == 0) body.addView(card("No subscriptions yet", "Add your Remnawave subscription URL to fetch entry nodes."));
        for (int i = 0; subs != null && i < subs.length(); i++) {
            final int idx = i;
            JSONObject sb = subs.optJSONObject(i);
            JSONArray nodes = sb.optJSONArray("nodes");
            String fetched = sb.optString("last_fetch");
            if (fetched.length() > 16) fetched = fetched.substring(0, 16).replace('T', ' ');
            boolean active = st.optBoolean("auto_select") || st.optInt("active_sub") == idx;
            LinearLayout item = card(sb.optString("name"), (nodes == null ? 0 : nodes.length()) + " nodes | " + fetched + (active ? " | Active" : ""));
            item.addView(row(btn("Use only this", false, () -> {
                Ctl.patch("active_sub", idx);
                Ctl.patch("auto_select", false);
            }), btn("Remove", false, () -> confirm("Remove " + sb.optString("name") + "?", () -> Ctl.cmd("rmsub", "index", idx)))));
            body.addView(item);
        }
        JSONObject s = Ctl.status;
        String pin = st.optString("pin_entry");
        LinearLayout nodes = card("Entry nodes", pin.isEmpty() ? "Automatic selection | tap Pin to force an entry" : "Pinned: " + pin);
        if (!pin.isEmpty()) nodes.addView(btn("Restore automatic selection", false, () -> Ctl.patch("pin_entry", "")));
        if (s != null) {
            pool(nodes, "Country | T2", s.optJSONArray("Country"), pin);
            pool(nodes, "Bypass | T3", s.optJSONArray("Bypass"), pin);
        }
        body.addView(nodes);
    }

    private void pool(LinearLayout parent, String title, JSONArray probes, String pin) {
        parent.addView(text(title, 16, ACCENT, true));
        for (int i = 0; probes != null && i < probes.length(); i++) {
            JSONObject p = probes.optJSONObject(i);
            String name = p.optString("Name");
            StringBuilder stats = new StringBuilder(p.optString("Server"));
            if (p.optBoolean("OK")) stats.append(" | ").append(p.optLong("Latency") / 1_000_000).append(" ms");
            else stats.append(" | Offline");
            if (p.optDouble("Speed") > 0) stats.append(" | ").append(Math.round(p.optDouble("Speed"))).append(" Mbps");
            if (p.optInt("Samples") > 0) stats.append(" | ").append(Math.round(p.optDouble("Flakiness"))).append("% flakiness");
            parent.addView(row(text(name + "\n" + stats, 12, TEXT, false),
                    btn(name.equals(pin) ? "Unpin" : "Pin", false, () -> Ctl.patch("pin_entry", name.equals(pin) ? "" : name))));
        }
    }

    private void renderMain(JSONObject st) {
        body.addView(btn("Add failover slot", true, () -> addSlot(st)));
        JSONArray slots = st.optJSONArray("slots");
        String tunSlot = tunSlot(st);
        JSONObject s = Ctl.status;
        for (int i = 0; slots != null && i < slots.length(); i++) {
            final int si = i;
            JSONObject sl = slots.optJSONObject(i);
            String name = sl.optString("name");
            String live = "";
            JSONArray liveSlots = s == null ? null : s.optJSONArray("Slots");
            for (int k = 0; liveSlots != null && k < liveSlots.length(); k++) {
                JSONObject current = liveSlots.optJSONObject(k);
                if (name.equals(current.optString("Name"))) live = " | " + line(current);
            }
            LinearLayout slot = card(name, "socks5://" + listenHost(st) + ":" + sl.optInt("port") + live + (name.equals(tunSlot) ? " | VPN slot" : ""));
            slot.addView(toggle("Slot enabled", sl.optBoolean("enabled"), on -> editSlot(st, si, sl, "enabled", on)));
            slot.addView(row(btn("Edit port", false, () -> editPort(st, si)),
                    btn("Use for VPN", false, () -> Ctl.patch("tun_slot", name)),
                    btn("Delete slot", false, () -> removeSlot(st, si))));
            JSONArray mains = sl.optJSONArray("mains");
            for (int m = 0; mains != null && m < mains.length(); m++) {
                final int mi = m;
                JSONObject mn = mains.optJSONObject(m);
                slot.addView(text(mn.optString("name") + "\n" + mainHost(mn.optString("url")) + " | " + proto(mn.optString("url")), 14, TEXT, true));
                slot.addView(toggle("Exit enabled", mn.optBoolean("enabled"), on -> editMain(st, si, mi, "enabled", on)));
                slot.addView(toggle("Allow direct | T1", mn.optBoolean("allow_no_hop"), on -> editMain(st, si, mi, "allow_no_hop", on)));
                slot.addView(btn("Delete final exit", false, () -> confirm("Delete " + mn.optString("name") + "?", () -> editMain(st, si, mi, "delete", true))));
            }
            slot.addView(btn("Add final exit", false, () -> askMain(st, si)));
            body.addView(slot);
        }
        body.addView(card("Routing compatibility", "Vision exits are direct-only. Plain exits can use any hop; REALITY exits require a non-Vision hop."));
    }

    private void renderLog() {
        LinearLayout logs = card("Daemon events", "Live engine activity | newest at the bottom");
        logs.addView(btn("Clear view", false, () -> { Core.clearLog(); logView.setText(""); }));
        logView = text("", 12, MUTED, false);
        logView.setTypeface(Typeface.MONOSPACE);
        logView.setTextIsSelectable(true);
        logs.addView(logView);
        body.addView(logs);
        logView.setText(Core.setListener(line -> runOnUiThread(() -> {
            if (tab == LOG && logView != null) logView.append(line + "\n");
        })));
        scroll.post(() -> scroll.fullScroll(View.FOCUS_DOWN));
    }

    private void renderConfig(JSONObject st) {
        String[] names = {"Network", "VPN", "Exit routes", "Providers & nodes", "Updates", "Device"};
        GridLayout sections = new GridLayout(this);
        sections.setColumnCount(2);
        for (int i = 0; i < names.length; i++) {
            final int selected = i;
            Button section = btn(names[i], i == configSection, () -> {
                configSection = selected;
                render();
            });
            section.setTextSize(13);
            section.setGravity(Gravity.CENTER);
            GridLayout.LayoutParams params = new GridLayout.LayoutParams();
            params.width = 0;
            params.height = dp(48);
            params.columnSpec = GridLayout.spec(GridLayout.UNDEFINED, 1, 1f);
            params.setMargins(dp(2), dp(2), dp(2), dp(2));
            sections.addView(section, params);
        }
        body.addView(sections);

        if (configSection == 0) {
            LinearLayout failover = card("Automatic failover", "Route selection, health checks, and recovery");
            intRow(failover, st, "Probe interval | seconds", "interval_s", 12, 2, 600);
            intRow(failover, st, "Egress timeout | seconds", "timeout_s", 6, 1, 60);
            intRow(failover, st, "Recovery threshold", "up_threshold", 3, 1, 10);
            intRow(failover, st, "Failure threshold", "down_threshold", 2, 1, 10);
            String[] tiers = {"Automatic", "T1 | Direct", "T2 | Country", "T3 | Bypass"};
            chooseRow(failover, "Pin tier", tiers[st.optInt("pin_tier")], "Pin tier", tiers, which -> Ctl.patch("pin_tier", which));
            boolRow(failover, st, "Force a hop | skip T1", "force_hop", false);
            body.addView(failover);

            LinearLayout network = card("Local listeners", "Restart the daemon to apply these changes");
            intRow(network, st, "Listen port", "listen_port", 2084, 1024, 65535);
            intRow(network, st, "First-hop port | 0 = auto", "entry_port", 0, 0, 65535);
            boolean lan = !"127.0.0.1".equals(st.optString("listen_addr"));
            actionRow(network, "Allow LAN connections", lan ? "0.0.0.0" : "127.0.0.1", () -> Ctl.patch("listen_addr", lan ? "127.0.0.1" : "0.0.0.0"));
            String[] levels = {"none", "error", "warning", "info", "debug"};
            String level = st.optString("log_level").isEmpty() ? "warning" : st.optString("log_level");
            chooseRow(network, "Xray log level", level, "Xray log level", levels, which -> Ctl.patch("log_level", levels[which]));
            body.addView(network);
        } else if (configSection == 1) {
            LinearLayout tun = card("System-wide VPN", "Static routed DNS | applied when the VPN reconnects");
            textRow(tun, st, "Static DNS resolver", "tun_static_dns", "8.8.8.8");
            boolRow(tun, st, "Answer ping locally", "tun_no_icmp", true);
            textRow(tun, st, "TUN interface address", "tun_addr", "198.18.0.1/30");
            intRow(tun, st, "TUN MTU | 0 = default", "tun_mtu", 0, 0, 9000);
            body.addView(tun);
        } else if (configSection == 2) {
            renderMain(st);
        } else if (configSection == 3) {
            renderSubs(st);
        } else if (configSection == 4) {
            LinearLayout updates = card("Subscription updates", "Background refresh and optional fetch proxy");
            boolRow(updates, st, "Automatic refresh", "no_auto_fetch", true);
            intRow(updates, st, "Refresh interval | hours", "fetch_interval_h", 24, 1, 168);
            boolRow(updates, st, "Use fetch proxy", "use_fetch_proxy", false);
            textRow(updates, st, "Fetch proxy", "fetch_proxy", "socks5://127.0.0.1:2084");
            body.addView(updates);
        } else {
            LinearLayout identity = card("Device identity", "Stable Happ-compatible identity for every panel");
            JSONObject device = st.optJSONObject("device");
            String hwid = device == null ? "" : device.optString("hwid");
            actionRow(identity, "Hardware ID", hwid, () -> ask("Hardware ID", hwid, InputType.TYPE_CLASS_TEXT, value -> {
                if (!value.matches("[a-zA-Z0-9=-]{10,64}")) { toast("HWID must be 10-64 letters, digits, = or -"); return; }
                try { Ctl.patch("device", new JSONObject(device.toString()).put("hwid", value)); } catch (Exception e) { failed(e); }
            }));
            if (device != null) {
                identity.addView(text("Panel OS: " + device.optString("os") + " " + device.optString("os_ver")
                        + "\nModel: " + device.optString("model") + "\nUser agent: " + device.optString("ua")
                        + "\nLocale: " + device.optString("locale"), 11, MUTED, false));
            }
            body.addView(identity);
        }
    }

    private void toggleVpn() {
        if (VpnSvc.running) {
            startService(new Intent(this, VpnSvc.class).setAction(VpnSvc.ACTION_STOP));
            return;
        }
        Intent prep = VpnService.prepare(this);
        if (prep != null) startActivityForResult(prep, 1);
        else onActivityResult(1, RESULT_OK, null);
    }

    @Override
    protected void onActivityResult(int request, int result, Intent data) {
        if (request != 1 || result != RESULT_OK) return;
        Intent intent = new Intent(this, VpnSvc.class).setAction(VpnSvc.ACTION_START);
        if (Build.VERSION.SDK_INT >= 26) startForegroundService(intent);
        else startService(intent);
    }

    private void addSub() {
        ask("Subscription URL", "", InputType.TYPE_TEXT_VARIATION_URI, value -> {
            if (!value.startsWith("http://") && !value.startsWith("https://")) { toast("Enter a valid http(s) URL"); return; }
            if (Ctl.cmd("addsub", "url", value)) fetching("adding subscription");
        });
    }

    private void addSlot(JSONObject st) {
        ask("New slot name", "slot" + (st.optJSONArray("slots").length() + 1), InputType.TYPE_CLASS_TEXT, name -> {
            try {
                JSONArray slots = copy(st.optJSONArray("slots"));
                for (int i = 0; i < slots.length(); i++) if (name.equals(slots.optJSONObject(i).optString("name"))) { toast("Slot name already exists"); return; }
                slots.put(new JSONObject().put("name", name).put("port", freePort(st)).put("enabled", true).put("mains", new JSONArray()));
                Ctl.patch("slots", slots);
            } catch (Exception e) { failed(e); }
        });
    }

    private void askMain(JSONObject st, int si) {
        ask("Final VLESS exit", "", InputType.TYPE_TEXT_VARIATION_URI, value -> {
            if (!value.startsWith("vless://")) { toast("Enter a valid vless:// URL"); return; }
            try {
                JSONArray slots = copy(st.optJSONArray("slots"));
                JSONObject sl = slots.optJSONObject(si);
                JSONArray mains = sl.optJSONArray("mains");
                if (mains == null) sl.put("mains", mains = new JSONArray());
                mains.put(new JSONObject().put("name", mainName(value)).put("url", value).put("enabled", true)
                        .put("allow_no_hop", value.contains("xtls-rprx-vision") || value.contains("security=reality")));
                Ctl.patch("slots", slots);
            } catch (Exception e) { failed(e); }
        });
    }

    private void editPort(JSONObject st, int si) {
        JSONArray slots = copy(st.optJSONArray("slots"));
        JSONObject sl = slots.optJSONObject(si);
        ask("Slot SOCKS5 port", "" + sl.optInt("port"), InputType.TYPE_CLASS_NUMBER, value -> {
            try {
                int port = Integer.parseInt(value);
                if (port < 1024 || port > 65535) throw new NumberFormatException();
                for (int i = 0; i < slots.length(); i++) {
                    int other = slots.optJSONObject(i).optInt("port");
                    if (i != si && (other == port || other == port + 1 || other + 1 == port)) { toast("Port is already used"); return; }
                }
                sl.put("port", port);
                Ctl.patch("slots", slots);
            } catch (NumberFormatException e) { toast("Port must be between 1024 and 65535"); }
            catch (Exception e) { failed(e); }
        });
    }

    private void removeSlot(JSONObject st, int si) {
        JSONArray slots = copy(st.optJSONArray("slots"));
        if (slots.length() <= 1) { toast("The last slot cannot be removed"); return; }
        confirm("Delete " + slots.optJSONObject(si).optString("name") + "?", () -> { slots.remove(si); Ctl.patch("slots", slots); });
    }

    private void editSlot(JSONObject st, int si, JSONObject original, String key, Object value) {
        try {
            JSONArray slots = copy(st.optJSONArray("slots"));
            slots.optJSONObject(si).put(key, value);
            Ctl.patch("slots", slots);
        } catch (Exception e) { failed(e); }
    }

    private void editMain(JSONObject st, int si, int mi, String key, Object value) {
        try {
            JSONArray slots = copy(st.optJSONArray("slots"));
            JSONArray mains = slots.optJSONObject(si).optJSONArray("mains");
            if ("delete".equals(key)) mains.remove(mi);
            else mains.optJSONObject(mi).put(key, value);
            Ctl.patch("slots", slots);
        } catch (Exception e) { failed(e); }
    }

    private void intRow(LinearLayout parent, JSONObject st, String label, String key, int fallback, int min, int max) {
        int current = st.optInt(key);
        if (current == 0 && fallback != 0) current = fallback;
        final int shown = current;
        actionRow(parent, label, "" + shown, () -> ask(label, "" + shown, InputType.TYPE_CLASS_NUMBER, value -> {
            try {
                int number = Integer.parseInt(value);
                if (number < min || number > max) { toast("Value must be between " + min + " and " + max); return; }
                Ctl.patch(key, number);
            } catch (NumberFormatException e) { toast("Enter a valid number"); }
        }));
    }

    private void textRow(LinearLayout parent, JSONObject st, String label, String key, String hint) {
        String current = st.optString(key);
        actionRow(parent, label, current.isEmpty() ? hint : current, () -> ask(label, current, InputType.TYPE_TEXT_VARIATION_URI, value -> Ctl.patch(key, value)));
    }

    private void boolRow(LinearLayout parent, JSONObject st, String label, String key, boolean inverted) {
        boolean on = st.optBoolean(key) != inverted;
        parent.addView(toggle(label, on, value -> Ctl.patch(key, value == inverted)));
    }

    private void chooseRow(LinearLayout parent, String label, String value, String title, String[] items, IntConsumer pick) {
        actionRow(parent, label, value, () -> choose(title, items, pick));
    }

    private void actionRow(LinearLayout parent, String label, String value, Runnable action) {
        LinearLayout row = row(text(label + "\n" + value, 13, TEXT, false), btn("Edit", false, action));
        parent.addView(row);
    }

    private Switch toggle(String label, boolean checked, Consumer<Boolean> changed) {
        Switch sw = new Switch(this);
        sw.setText(label);
        sw.setTextColor(TEXT);
        sw.setTextSize(13);
        sw.setPadding(0, dp(4), 0, dp(4));
        sw.setChecked(checked);
        if (Build.VERSION.SDK_INT >= 21) {
            sw.setThumbTintList(new ColorStateList(new int[][]{{android.R.attr.state_checked}, {}}, new int[]{ACCENT, MUTED}));
            sw.setTrackTintList(new ColorStateList(new int[][]{{android.R.attr.state_checked}, {}}, new int[]{Color.rgb(36, 103, 89), Color.rgb(51, 66, 88)}));
        }
        sw.setOnCheckedChangeListener((view, on) -> changed.accept(on));
        return sw;
    }

    private LinearLayout card(String title, String subtitle) {
        LinearLayout card = new LinearLayout(this);
        card.setOrientation(LinearLayout.VERTICAL);
        card.setPadding(dp(12), dp(10), dp(12), dp(10));
        card.setBackground(round(CARD, 10));
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(-1, -2);
        params.setMargins(0, 0, 0, dp(8));
        card.setLayoutParams(params);
        card.addView(text(title, 15, TEXT, true));
        if (subtitle != null && !subtitle.isEmpty()) card.addView(text(subtitle, 11, MUTED, false));
        return card;
    }

    private TextView text(String value, int size, int color, boolean bold) {
        TextView view = new TextView(this);
        view.setText(value);
        view.setTextSize(size);
        view.setTextColor(color);
        view.setLineSpacing(0, 1.05f);
        view.setPadding(0, dp(2), 0, dp(2));
        if (bold) view.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        return view;
    }

    private Button btn(String label, boolean primary, Runnable action) {
        Button button = new Button(this);
        button.setText(label);
        button.setAllCaps(false);
        button.setTextSize(12);
        button.setTextColor(primary ? BG : TEXT);
        button.setMinHeight(dp(34));
        button.setMinimumHeight(dp(34));
        button.setPadding(dp(11), 0, dp(11), 0);
        GradientDrawable content = round(primary ? ACCENT : Color.rgb(31, 39, 49), 8);
        button.setBackground(new RippleDrawable(ColorStateList.valueOf(primary ? 0x55ffffff : 0x3368d5bd), content, round(Color.WHITE, 8)));
        button.setOnClickListener(view -> action.run());
        return button;
    }

    private void loadBranding() {
        try (java.io.InputStream stream = getAssets().open("branding/tokens.json")) {
            java.io.ByteArrayOutputStream output = new java.io.ByteArrayOutputStream();
            byte[] buffer = new byte[4096];
            int count;
            while ((count = stream.read(buffer)) != -1) output.write(buffer, 0, count);
            JSONObject colors = new JSONObject(output.toString("UTF-8")).getJSONObject("colors");
            BG = Color.parseColor(colors.getString("background"));
            CARD = Color.parseColor(colors.getString("surface"));
            TEXT = Color.parseColor(colors.getString("text"));
            MUTED = Color.parseColor(colors.getString("muted"));
            ACCENT = Color.parseColor(colors.getString("accent"));
            DANGER = Color.parseColor(colors.getString("danger"));
        } catch (Exception error) {
            android.util.Log.w("Branding", "Using built-in colors", error);
        }
    }

    private Bitmap loadBitmap(String name) {
        try (java.io.InputStream stream = getAssets().open("branding/png/" + name + ".png")) {
            return BitmapFactory.decodeStream(stream);
        } catch (Exception error) {
            android.util.Log.w("Branding", "Could not load " + name, error);
            return null;
        }
    }

    private void setFooterIcon(Button button, String name, int tint) {
        Bitmap bitmap = loadBitmap(name);
        if (bitmap == null) return;
        BitmapDrawable icon = new BitmapDrawable(getResources(), bitmap);
        icon.setTint(tint);
        icon.setBounds(0, 0, dp(21), dp(21));
        button.setCompoundDrawables(null, icon, null, null);
        button.setCompoundDrawablePadding(dp(2));
    }

    private void styleTab(Button button, boolean active) {
        int color = active ? ACCENT : MUTED;
        button.setTextColor(color);
        GradientDrawable content = round(active ? Color.rgb(27, 37, 44) : Color.TRANSPARENT, 10);
        button.setBackground(new RippleDrawable(ColorStateList.valueOf(0x3368d5bd), content, round(Color.WHITE, 10)));
        for (int i = 0; i < tabBtns.length; i++) {
            if (tabBtns[i] == button) setFooterIcon(button, ICONS[i], color);
        }
    }

    private LinearLayout row(View... views) {
        LinearLayout row = new LinearLayout(this);
        row.setGravity(Gravity.CENTER_VERTICAL);
        for (int i = 0; i < views.length; i++) {
            View view = views[i];
            LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(i == 0 ? 0 : -2, -2);
            if (i == 0) params.weight = 1;
            params.setMargins(dp(2), dp(2), dp(2), dp(2));
            row.addView(view, params);
        }
        return row;
    }

    private void addMetric(LinearLayout row, String name, String value) {
        LinearLayout metric = new LinearLayout(this);
        metric.setOrientation(LinearLayout.VERTICAL);
        metric.setPadding(dp(7), dp(7), dp(7), dp(7));
        metric.setBackground(round(Color.rgb(19, 24, 31), 8));
        metric.addView(text(name, 10, MUTED, false));
        metric.addView(text(value, 14, TEXT, true));
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(0, -2, 1);
        params.setMargins(dp(2), dp(4), dp(2), 0);
        row.addView(metric, params);
    }

    private GradientDrawable round(int color, int radius) {
        GradientDrawable shape = new GradientDrawable();
        shape.setColor(color);
        shape.setCornerRadius(dp(radius));
        return shape;
    }

    private View spacer(int height) {
        View view = new View(this);
        view.setLayoutParams(new LinearLayout.LayoutParams(1, dp(height)));
        return view;
    }

    private void ask(String title, String initial, int variation, Consumer<String> ok) {
        EditText input = new EditText(this);
        input.setInputType(variation == InputType.TYPE_CLASS_NUMBER ? variation : InputType.TYPE_CLASS_TEXT | variation);
        input.setText(initial);
        input.setTextColor(TEXT);
        input.setHintTextColor(MUTED);
        int pad = dp(18);
        input.setPadding(pad, pad, pad, pad);
        new AlertDialog.Builder(this).setTitle(title).setView(input)
                .setPositiveButton("Save", (dialog, which) -> {
                    String value = input.getText().toString().trim();
                    if (!value.isEmpty() && !Ctl.connected) toast("Daemon not connected");
                    else if (!value.isEmpty() || title.contains("proxy") || title.contains("DNS") || title.contains("TUN interface")) ok.accept(value);
                })
                .setNegativeButton("Cancel", null).show();
    }

    private void choose(String title, String[] items, IntConsumer pick) {
        new AlertDialog.Builder(this).setTitle(title).setItems(items, (dialog, which) -> pick.accept(which)).show();
    }

    private void confirm(String question, Runnable yes) {
        new AlertDialog.Builder(this).setMessage(question).setPositiveButton("Confirm", (dialog, which) -> yes.run()).setNegativeButton("Cancel", null).show();
    }

    private void copy(String value) {
        ClipboardManager clipboard = getSystemService(ClipboardManager.class);
        clipboard.setPrimaryClip(ClipData.newPlainText("SOCKS5 address", value));
        toast("Copied to clipboard");
    }

    private static String tunSlot(JSONObject st) {
        JSONArray slots = st.optJSONArray("slots");
        String wanted = st.optString("tun_slot"), first = "";
        for (int i = 0; slots != null && i < slots.length(); i++) {
            JSONObject slot = slots.optJSONObject(i);
            if (!slot.optBoolean("enabled")) continue;
            if (first.isEmpty()) first = slot.optString("name");
            if (slot.optString("name").equals(wanted)) return wanted;
        }
        return first;
    }

    private static String listenHost(JSONObject st) {
        String address = st.optString("listen_addr");
        return address.isEmpty() ? "0.0.0.0" : address;
    }

    private static int freePort(JSONObject st) {
        int base = st.optInt("listen_port", 2084);
        List<Integer> used = new ArrayList<>();
        used.add(base); used.add(base + 1);
        JSONArray slots = st.optJSONArray("slots");
        for (int i = 0; slots != null && i < slots.length(); i++) {
            int port = slots.optJSONObject(i).optInt("port");
            used.add(port); used.add(port + 1);
        }
        for (int port = base + 2; port < 65535; port++) if (!used.contains(port) && !used.contains(port + 1)) return port;
        return 0;
    }

    private static String okCount(JSONArray probes) {
        int ok = 0;
        for (int i = 0; probes != null && i < probes.length(); i++) if (probes.optJSONObject(i).optBoolean("OK")) ok++;
        return ok + "/" + (probes == null ? 0 : probes.length());
    }

    private static boolean anyMain(JSONObject st) {
        JSONArray slots = st.optJSONArray("slots");
        for (int i = 0; slots != null && i < slots.length(); i++) {
            JSONArray mains = slots.optJSONObject(i).optJSONArray("mains");
            if (mains != null && mains.length() > 0) return true;
        }
        return false;
    }

    private static String proto(String value) {
        android.net.Uri uri = android.net.Uri.parse(value);
        String flow = uri.getQueryParameter("flow"), security = uri.getQueryParameter("security"), network = uri.getQueryParameter("type");
        String tag = flow != null && flow.contains("xtls-rprx-vision") ? "vision" : security == null || security.isEmpty() || security.equals("none") ? "plain" : security;
        return network == null || network.isEmpty() || network.equals("tcp") ? tag : tag + " | " + network;
    }

    private static String mainName(String value) {
        android.net.Uri uri = android.net.Uri.parse(value);
        String fragment = uri.getFragment();
        return fragment != null && !fragment.trim().isEmpty() ? fragment.trim() : uri.getHost();
    }

    private static String mainHost(String value) {
        android.net.Uri uri = android.net.Uri.parse(value);
        return uri.getHost() == null ? "Invalid VLESS URL" : uri.getHost() + (uri.getPort() > 0 ? ":" + uri.getPort() : "");
    }

    private static JSONArray copy(JSONArray value) {
        try { return new JSONArray(value == null ? "[]" : value.toString()); } catch (Exception e) { return new JSONArray(); }
    }

    private void fetching(String what) {
        Core.log("-> " + what + " ...");
        toast("Fetching... results appear in Activity");
    }

    private void failed(Exception error) {
        Core.log("Edit failed: " + error);
        toast("Edit failed: " + error.getMessage());
    }

    private void toast(String value) {
        Toast.makeText(this, value, Toast.LENGTH_SHORT).show();
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
