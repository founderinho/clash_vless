package com.clashvless;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.graphics.Typeface;
import android.net.VpnService;
import android.os.Build;
import android.os.Bundle;
import android.text.InputType;
import android.view.View;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;
import java.util.function.Consumer;
import java.util.function.IntConsumer;

/** Bare dashboard mirroring the TUI tabs: Status / Subs / Main / Log / Config. All edits go to the daemon live. */
public class MainActivity extends Activity {
    private static final String[] TABS = {"Status", "Subs", "Main", "Log", "Config"};
    private static final int STATUS = 0, SUBS = 1, MAIN = 2, LOG = 3, CONFIG = 4;

    private int tab;
    private TextView head;
    private Button vpnBtn;
    private final Button[] tabBtns = new Button[TABS.length];
    private LinearLayout body;
    private ScrollView scroll;
    private TextView logView;

    @Override
    protected void onCreate(Bundle b) {
        super.onCreate(b);
        if (Build.VERSION.SDK_INT >= 33) requestPermissions(new String[]{"android.permission.POST_NOTIFICATIONS"}, 0);

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(16, 16, 16, 16);

        LinearLayout top = new LinearLayout(this);
        top.setBaselineAligned(false); // else the wrapped status line is clipped to the button's baseline
        top.setGravity(android.view.Gravity.CENTER_VERTICAL);
        head = mono("");
        head.setTypeface(Typeface.MONOSPACE, Typeface.BOLD);
        top.addView(head, new LinearLayout.LayoutParams(0, -2, 1));
        vpnBtn = btn("Connect", this::toggleVpn);
        top.addView(vpnBtn);
        root.addView(top);

        LinearLayout tabs = new LinearLayout(this);
        for (int i = 0; i < TABS.length; i++) {
            final int t = i;
            tabBtns[i] = btn(TABS[i], () -> { tab = t; render(); });
            tabs.addView(tabBtns[i], new LinearLayout.LayoutParams(0, -2, 1));
        }
        root.addView(tabs);

        scroll = new ScrollView(this);
        body = new LinearLayout(this);
        body.setOrientation(LinearLayout.VERTICAL);
        scroll.addView(body);
        root.addView(scroll, new LinearLayout.LayoutParams(-1, 0, 1));
        setContentView(root);

        Ctl.start(this);
    }

    @Override
    protected void onStart() {
        super.onStart();
        Core.uiVisible = true;
        Ctl.listener = stateChanged -> runOnUiThread(() -> {
            if (tab == STATUS || tab == SUBS || tab == MAIN || stateChanged) render();
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

    // --- rendering ---------------------------------------------------------------

    private void render() {
        renderHead();
        for (int i = 0; i < TABS.length; i++) tabBtns[i].setText(i == tab ? "[" + TABS[i] + "]" : TABS[i]);
        body.removeAllViews();
        if (tab != LOG) Core.setListener(null);
        JSONObject st = Ctl.state;
        if (tab != LOG && (!Ctl.connected || st == null)) {
            body.addView(mono("connecting to daemon…"));
            return;
        }
        switch (tab) {
            case STATUS: renderStatus(st); break;
            case SUBS: renderSubs(st); break;
            case MAIN: renderMain(st); break;
            case LOG: renderLog(); break;
            case CONFIG: renderConfig(st); break;
        }
    }

    private void renderHead() {
        JSONObject s = Ctl.status;
        head.setText((VpnSvc.running ? "VPN on  " : "VPN off  ") + (s == null ? "" : line(s)));
        vpnBtn.setText(VpnSvc.running ? "Disconnect" : "Connect");
    }

    private static String line(JSONObject s) {
        int tier = s.optInt("Tier");
        long ms = s.optLong("Egress") / 1_000_000;
        String main = s.optString("Main");
        if (main.isEmpty()) main = "main";
        String note = s.optString("Note");
        note = note.isEmpty() ? "" : "  " + note;
        if (tier == 0) return "✖ DOWN " + s.optString("Err") + note;
        if (tier == 1) return "● T1 direct → " + main + "  " + ms + "ms" + note;
        return "● T" + tier + " " + s.optString("Entry") + " → " + main + "  " + ms + "ms" + note;
    }

    private void renderStatus(JSONObject st) {
        JSONObject s = Ctl.status;
        String tunSlot = tunSlot(st);
        body.addView(mono(VpnSvc.running ? "VPN on — all traffic via slot \"" + tunSlot + "\""
                : "VPN off — slots still serve SOCKS on " + listenHost(st)));
        if (s == null) return;
        if (!s.optString("TunErr").isEmpty()) body.addView(mono(s.optString("TunErr")));
        body.addView(section("Slots"));
        JSONArray slots = s.optJSONArray("Slots");
        for (int i = 0; slots != null && i < slots.length(); i++) {
            JSONObject sl = slots.optJSONObject(i);
            body.addView(mono((sl.optBoolean("IsTun") ? "◉ " : "○ ") + sl.optString("Name") + " :" + sl.optInt("Port") + "\n   " + line(sl)));
        }
        body.addView(section("Entry pools"));
        body.addView(mono("Country (T2): " + okCount(s.optJSONArray("Country")) + "\nBypass  (T3): " + okCount(s.optJSONArray("Bypass"))));
        String pin = st.optString("pin_entry");
        int pinTier = st.optInt("pin_tier");
        body.addView(mono("pin: tier " + (pinTier == 0 ? "auto" : "T" + pinTier) + " · entry " + (pin.isEmpty() ? "auto" : pin)
                + (st.optBoolean("force_hop") ? " · force-hop" : "")));
        body.addView(btn("Recheck now", () -> Ctl.cmd("kick")));
    }

    private void renderSubs(JSONObject st) {
        boolean auto = st.optBoolean("auto_select");
        int active = st.optInt("active_sub");
        body.addView(row(
                btn("+ Sub", () -> ask("Subscription URL", "", InputType.TYPE_TEXT_VARIATION_URI, u -> {
                    if (Ctl.cmd("addsub", "url", u)) fetching("adding " + u);
                })),
                btn("Fetch all", () -> { if (Ctl.cmd("refetch")) fetching("refetching all subs"); }),
                btn(auto ? "Mode: all" : "Mode: one", () -> Ctl.patch("auto_select", !auto))));
        JSONArray subs = st.optJSONArray("subs");
        for (int i = 0; subs != null && i < subs.length(); i++) {
            final int idx = i;
            JSONObject sb = subs.optJSONObject(i);
            JSONArray nodes = sb.optJSONArray("nodes");
            String fetched = sb.optString("last_fetch");
            if (fetched.length() > 16) fetched = fetched.substring(0, 16).replace('T', ' ');
            String mark = auto || idx == active ? "► " : "  ";
            body.addView(tap(mono(mark + "[" + i + "] " + sb.optString("name") + " — " + (nodes == null ? 0 : nodes.length())
                    + " nodes · " + fetched), () -> choose(sb.optString("name"), new String[]{"Use only this sub", "Remove"}, w -> {
                if (w == 0) {
                    Ctl.patch("active_sub", idx);
                    Ctl.patch("auto_select", false);
                } else {
                    confirm("Remove " + sb.optString("name") + "?", () -> Ctl.cmd("rmsub", "index", idx));
                }
            })));
        }
        JSONObject s = Ctl.status;
        String pin = st.optString("pin_entry");
        body.addView(section("Entry nodes — tap to pin/unpin" + (pin.isEmpty() ? "" : " (★ " + pin + ")")));
        if (s == null) return;
        pool("Country (T2)", s.optJSONArray("Country"), pin);
        pool("Bypass (T3)", s.optJSONArray("Bypass"), pin);
    }

    private void pool(String title, JSONArray probes, String pin) {
        body.addView(mono(title));
        for (int i = 0; probes != null && i < probes.length(); i++) {
            JSONObject p = probes.optJSONObject(i);
            String name = p.optString("Name");
            StringBuilder t = new StringBuilder(name.equals(pin) ? "★ " : p.optBoolean("OK") ? "● " : "✖ ").append(name);
            if (p.optBoolean("OK")) t.append("  ").append(p.optLong("Latency") / 1_000_000).append("ms");
            if (p.optDouble("Speed") > 0) t.append("  ").append(Math.round(p.optDouble("Speed"))).append("Mbps");
            if (p.optInt("Samples") > 0) t.append("  flake ").append(Math.round(p.optDouble("Flakiness"))).append("%");
            body.addView(tap(mono("  " + t), () -> Ctl.patch("pin_entry", name.equals(pin) ? "" : name)));
        }
    }

    private void renderMain(JSONObject st) {
        body.addView(row(btn("+ Slot", () -> addSlot(st))));
        JSONArray slots = st.optJSONArray("slots");
        String tunSlot = tunSlot(st);
        JSONObject s = Ctl.status;
        for (int i = 0; slots != null && i < slots.length(); i++) {
            final int si = i;
            JSONObject sl = slots.optJSONObject(i);
            String name = sl.optString("name");
            String live = "";
            JSONArray ss = s == null ? null : s.optJSONArray("Slots");
            for (int k = 0; ss != null && k < ss.length(); k++)
                if (name.equals(ss.optJSONObject(k).optString("Name"))) live = "\n   " + line(ss.optJSONObject(k));
            body.addView(tap(mono((name.equals(tunSlot) ? "◉ " : "○ ") + "slot \"" + name + "\" :" + sl.optInt("port")
                    + (sl.optBoolean("enabled") ? " [on]" : " [off]") + live), () -> slotActions(st, si)));
            JSONArray mains = sl.optJSONArray("mains");
            for (int m = 0; mains != null && m < mains.length(); m++) {
                final int mi = m;
                JSONObject mn = mains.optJSONObject(m);
                String url = mn.optString("url");
                body.addView(tap(mono("    [" + m + "] " + (mn.optBoolean("enabled") ? "✓ " : "✗ ") + mn.optString("name")
                        + "  " + (mn.optBoolean("allow_no_hop") ? "direct-ok" : "hop-only") + "  " + proto(url)), () -> mainActions(st, si, mi)));
            }
        }
        body.addView(mono("\nVision mains are direct-only (T1). Plain mains hop through any entry; reality mains only through non-Vision entries."));
    }

    private void renderLog() {
        body.addView(row(btn("Clear", () -> { Core.clearLog(); logView.setText(""); })));
        logView = mono("");
        logView.setTextSize(10);
        logView.setTextIsSelectable(true);
        body.addView(logView);
        logView.setText(Core.setListener(l -> runOnUiThread(() -> {
            if (tab == LOG && logView != null) logView.append(l + "\n");
        })));
        scroll.post(() -> scroll.fullScroll(View.FOCUS_DOWN));
    }

    private void renderConfig(JSONObject st) {
        intRow(st, "Probe interval (s)", "interval_s", 12);
        intRow(st, "Egress timeout (s)", "timeout_s", 6);
        intRow(st, "Up threshold", "up_threshold", 3);
        intRow(st, "Down threshold", "down_threshold", 2);
        int pt = st.optInt("pin_tier");
        cfg("Pin tier", pt == 0 ? "auto" : "T" + pt, () -> choose("Pin tier", new String[]{"auto", "T1", "T2", "T3"}, w -> Ctl.patch("pin_tier", w)));
        boolRow(st, "Force hop (skip T1)", "force_hop", false);
        boolRow(st, "Auto-fetch subs", "no_auto_fetch", true);
        intRow(st, "Auto-fetch every (h)", "fetch_interval_h", 24);
        boolRow(st, "Use fetch proxy", "use_fetch_proxy", false);
        String fp = st.optString("fetch_proxy");
        cfg("Fetch proxy", fp.isEmpty() ? "(unset)" : fp, () -> ask("Fetch proxy (socks5://h:p, http://h:p, h:p)", fp,
                InputType.TYPE_TEXT_VARIATION_URI, v -> Ctl.patch("fetch_proxy", v)));
        String lvl = st.optString("log_level").isEmpty() ? "warning" : st.optString("log_level");
        String[] lvls = {"none", "error", "warning", "info", "debug"};
        cfg("Log level *", lvl, () -> choose("xray log level", lvls, w -> Ctl.patch("log_level", lvls[w])));
        boolean lan = "0.0.0.0".equals(st.optString("listen_addr"));
        cfg("Allow LAN (0.0.0.0) *", lan ? "on" : "off", () -> Ctl.patch("listen_addr", lan ? "127.0.0.1" : "0.0.0.0"));
        boolRow(st, "TUN ICMP ping *", "tun_no_icmp", true);
        JSONObject dev = st.optJSONObject("device");
        String hwid = dev == null ? "" : dev.optString("hwid");
        cfg("Device HWID", hwid, () -> ask("HWID (paste one from another device to share its panel slot)", hwid, InputType.TYPE_CLASS_TEXT, v -> {
            if (!v.matches("[a-zA-Z0-9=-]{10,64}")) { toast("HWID must be 10–64 chars: a-z A-Z 0-9 = -"); return; }
            try { Ctl.patch("device", new JSONObject(dev.toString()).put("hwid", v)); } catch (Exception ignored) { }
        }));
        body.addView(mono("\n* applies on next Connect/Disconnect (daemon restart).\nCLI: adb shell run-as com.clashvless sh files/cv <cmd>"));
    }

    // --- actions -----------------------------------------------------------------

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
    protected void onActivityResult(int req, int res, Intent data) {
        if (req != 1 || res != RESULT_OK) return;
        Intent i = new Intent(this, VpnSvc.class).setAction(VpnSvc.ACTION_START);
        if (Build.VERSION.SDK_INT >= 26) startForegroundService(i);
        else startService(i);
    }

    private void addSlot(JSONObject st) {
        JSONArray slots = copy(st.optJSONArray("slots"));
        ask("New slot name", "slot" + (slots.length() + 1), InputType.TYPE_CLASS_TEXT, name -> {
            try {
                slots.put(new JSONObject().put("name", name).put("port", freePort(st)).put("enabled", true).put("mains", new JSONArray()));
                Ctl.patch("slots", slots);
            } catch (Exception ignored) { }
        });
    }

    private void slotActions(JSONObject st, int si) {
        JSONArray slots = copy(st.optJSONArray("slots"));
        JSONObject sl = slots.optJSONObject(si);
        String name = sl.optString("name");
        choose("slot " + name, new String[]{sl.optBoolean("enabled") ? "Disable" : "Enable", "Use for VPN", "Port…", "Add main…", "Delete slot"}, w -> {
            try {
                switch (w) {
                    case 0: sl.put("enabled", !sl.optBoolean("enabled")); Ctl.patch("slots", slots); break;
                    case 1: Ctl.patch("tun_slot", name); break;
                    case 2: ask("Port (slot uses port and port+1)", "" + sl.optInt("port"), InputType.TYPE_CLASS_NUMBER, v -> {
                        try { sl.put("port", Integer.parseInt(v)); Ctl.patch("slots", slots); } catch (Exception e) { toast("bad port"); }
                    }); break;
                    case 3: ask("vless:// URL", "", InputType.TYPE_TEXT_VARIATION_URI, u -> {
                        if (!u.startsWith("vless://")) { toast("need a vless:// URL"); return; }
                        try {
                            sl.getJSONArray("mains").put(new JSONObject().put("name", mainName(u)).put("url", u)
                                    .put("enabled", true).put("allow_no_hop", u.contains("xtls-rprx-vision")));
                            Ctl.patch("slots", slots);
                        } catch (Exception ignored) { }
                    }); break;
                    case 4: confirm("Delete slot " + name + "?", () -> { slots.remove(si); Ctl.patch("slots", slots); }); break;
                }
            } catch (Exception ignored) { }
        });
    }

    private void mainActions(JSONObject st, int si, int mi) {
        JSONArray slots = copy(st.optJSONArray("slots"));
        JSONArray mains = slots.optJSONObject(si).optJSONArray("mains");
        JSONObject mn = mains.optJSONObject(mi);
        choose(mn.optString("name"), new String[]{mn.optBoolean("enabled") ? "Disable" : "Enable",
                mn.optBoolean("allow_no_hop") ? "Hop only (no T1)" : "Allow direct (T1)", "Delete"}, w -> {
            try {
                switch (w) {
                    case 0: mn.put("enabled", !mn.optBoolean("enabled")); break;
                    case 1: mn.put("allow_no_hop", !mn.optBoolean("allow_no_hop")); break;
                    case 2: mains.remove(mi); break;
                }
                Ctl.patch("slots", slots);
            } catch (Exception ignored) { }
        });
    }

    // --- helpers -----------------------------------------------------------------

    private static String tunSlot(JSONObject st) {
        JSONArray slots = st.optJSONArray("slots");
        String want = st.optString("tun_slot"), first = "";
        for (int i = 0; slots != null && i < slots.length(); i++) {
            JSONObject sl = slots.optJSONObject(i);
            if (!sl.optBoolean("enabled")) continue;
            if (first.isEmpty()) first = sl.optString("name");
            if (sl.optString("name").equals(want)) return want;
        }
        return first;
    }

    private static String listenHost(JSONObject st) {
        String a = st.optString("listen_addr");
        return a.isEmpty() ? "127.0.0.1" : a;
    }

    /** Lowest p with p and p+1 free (each slot also serves its first hop on Port+1), like store.freeSlotPort. */
    private static int freePort(JSONObject st) {
        int base = st.optInt("listen_port", 2084);
        List<Integer> used = new ArrayList<>();
        used.add(base);
        used.add(base + 1);
        JSONArray slots = st.optJSONArray("slots");
        for (int i = 0; slots != null && i < slots.length(); i++) {
            int p = slots.optJSONObject(i).optInt("port");
            used.add(p);
            used.add(p + 1);
        }
        for (int p = base + 2; p < 65535; p++) if (!used.contains(p) && !used.contains(p + 1)) return p;
        return 0;
    }

    private static String okCount(JSONArray probes) {
        int ok = 0, n = probes == null ? 0 : probes.length();
        for (int i = 0; i < n; i++) if (probes.optJSONObject(i).optBoolean("OK")) ok++;
        return ok + "/" + n + " up";
    }

    private static String proto(String u) {
        android.net.Uri p = android.net.Uri.parse(u);
        String flow = p.getQueryParameter("flow"), sec = p.getQueryParameter("security"), net = p.getQueryParameter("type");
        String tag = flow != null && flow.contains("xtls-rprx-vision") ? "vision" : sec == null || sec.isEmpty() || sec.equals("none") ? "plain" : sec;
        return net == null || net.isEmpty() || net.equals("tcp") ? tag : tag + "·" + net;
    }

    private static String mainName(String u) {
        android.net.Uri p = android.net.Uri.parse(u);
        String f = p.getFragment();
        return f != null && !f.trim().isEmpty() ? f.trim() : p.getHost();
    }

    private static JSONArray copy(JSONArray a) {
        try { return new JSONArray(a == null ? "[]" : a.toString()); } catch (Exception e) { return new JSONArray(); }
    }

    private void intRow(JSONObject st, String label, String key, int def) {
        int v = st.optInt(key);
        if (v == 0) v = def;
        final int cur = v;
        cfg(label, "" + cur, () -> ask(label, "" + cur, InputType.TYPE_CLASS_NUMBER, s -> {
            try { Ctl.patch(key, Integer.parseInt(s)); } catch (NumberFormatException e) { toast("not a number"); }
        }));
    }

    /** inverted = the stored key is the negation of the shown setting (no_auto_fetch, tun_no_icmp). */
    private void boolRow(JSONObject st, String label, String key, boolean inverted) {
        boolean on = st.optBoolean(key) != inverted;
        cfg(label, on ? "on" : "off", () -> Ctl.patch(key, on == inverted));
    }

    private void cfg(String label, String value, Runnable onTap) {
        body.addView(tap(mono(String.format("%-24s %s", label, value)), onTap));
    }

    private TextView section(String s) {
        TextView t = mono("\n" + s);
        t.setTypeface(Typeface.MONOSPACE, Typeface.BOLD);
        return t;
    }

    private TextView mono(String s) {
        TextView t = new TextView(this);
        t.setTypeface(Typeface.MONOSPACE);
        t.setTextSize(12);
        t.setText(s);
        t.setPadding(0, 6, 0, 6);
        return t;
    }

    private View tap(View v, Runnable r) {
        v.setOnClickListener(x -> r.run());
        v.setBackgroundResource(android.R.drawable.list_selector_background);
        return v;
    }

    private Button btn(String label, Runnable r) {
        Button b = new Button(this);
        b.setText(label);
        b.setAllCaps(false);
        b.setTextSize(12);
        b.setMinHeight(0);
        b.setMinimumHeight(0);
        b.setOnClickListener(v -> r.run());
        return b;
    }

    private LinearLayout row(View... vs) {
        LinearLayout r = new LinearLayout(this);
        for (View v : vs) r.addView(v, new LinearLayout.LayoutParams(0, -2, 1));
        return r;
    }

    private void ask(String title, String initial, int variation, Consumer<String> ok) {
        EditText e = new EditText(this);
        e.setInputType(variation == InputType.TYPE_CLASS_NUMBER ? variation : InputType.TYPE_CLASS_TEXT | variation);
        e.setText(initial);
        new AlertDialog.Builder(this).setTitle(title).setView(e)
                .setPositiveButton("OK", (d, w) -> {
                    String v = e.getText().toString().trim();
                    if (!v.isEmpty() && !Ctl.connected) toast("daemon not connected");
                    else if (!v.isEmpty()) ok.accept(v);
                })
                .setNegativeButton("Cancel", null).show();
    }

    private void choose(String title, String[] items, IntConsumer pick) {
        new AlertDialog.Builder(this).setTitle(title).setItems(items, (d, w) -> pick.accept(w)).show();
    }

    private void confirm(String q, Runnable yes) {
        new AlertDialog.Builder(this).setMessage(q).setPositiveButton("Yes", (d, w) -> yes.run()).setNegativeButton("No", null).show();
    }

    /** The sub only appears once its fetch ends (up to ~30s); say so, and where the result lands. */
    private void fetching(String what) {
        Core.log("→ " + what + " … (result below when the fetch ends)");
        toast("fetching… result shows in the Log tab");
    }

    private void toast(String s) {
        Toast.makeText(this, s, Toast.LENGTH_SHORT).show();
    }
}
