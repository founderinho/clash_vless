package com.clashvless;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Typeface;
import android.net.VpnService;
import android.os.Build;
import android.os.Bundle;
import android.view.View;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.regex.Pattern;

/** Bare-bones front end: an input box, a few buttons, and the daemon log. */
public class MainActivity extends Activity {
    private static final Pattern STATUS = Pattern.compile("^\\[\\d\\d:\\d\\d:\\d\\d] .*");
    private static final List<String> READ_ONLY = Arrays.asList("list", "subs", "whoami", "version", "status", "gen", "help");

    private TextView status, logView;
    private ScrollView scroll;
    private EditText input;

    @Override
    protected void onCreate(Bundle b) {
        super.onCreate(b);
        Core.writeCli(this, Core.dns(this));
        if (Build.VERSION.SDK_INT >= 33) requestPermissions(new String[]{"android.permission.POST_NOTIFICATIONS"}, 0);

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(24, 24, 24, 24);

        status = new TextView(this);
        status.setTypeface(Typeface.MONOSPACE, Typeface.BOLD);
        status.setText(VpnSvc.running ? "VPN on" : "VPN off");
        root.addView(status);

        input = new EditText(this);
        input.setHint("sub URL · vless:// · or a CLI command (e.g. slot, tun icmp off)");
        input.setTextSize(13);
        input.setMaxLines(4);
        root.addView(input);

        root.addView(row(
                btn("Add sub", v -> cmd(text("add"))),
                btn("Add main", v -> cmd(text("main", "add"))),
                btn("Fetch", v -> cmd(Arrays.asList("fetch"))),
                btn("Run", v -> cmd(text()))));
        root.addView(row(
                btn("Start", v -> startVpn()),
                btn("Stop", v -> stopVpn()),
                btn("Info", v -> info()),
                btn("Clear", v -> logView.setText(""))));

        scroll = new ScrollView(this);
        logView = new TextView(this);
        logView.setTypeface(Typeface.MONOSPACE);
        logView.setTextSize(11);
        logView.setTextIsSelectable(true);
        scroll.addView(logView);
        root.addView(scroll, new LinearLayout.LayoutParams(-1, 0, 1));
        setContentView(root);
    }

    @Override
    protected void onResume() {
        super.onResume();
        String backlog = Core.setListener(line -> runOnUiThread(() -> append(line)));
        logView.setText(backlog);
        for (String l : backlog.split("\n")) if (STATUS.matcher(l).matches()) status.setText(l);
        scroll.post(() -> scroll.fullScroll(View.FOCUS_DOWN));
    }

    @Override
    protected void onPause() {
        Core.setListener(null);
        super.onPause();
    }

    private void append(String line) {
        logView.append(line + "\n");
        if (STATUS.matcher(line).matches()) status.setText(line);
        else if (line.startsWith("■ VPN off")) status.setText("VPN off");
        scroll.post(() -> scroll.fullScroll(View.FOCUS_DOWN));
    }

    private List<String> text(String... prefix) {
        List<String> a = new ArrayList<>(Arrays.asList(prefix));
        for (String s : input.getText().toString().trim().split("\\s+")) if (!s.isEmpty()) a.add(s);
        return a;
    }

    /** Runs a CLI command. The daemon owns state.json while running, so edits stop it first and restart after. */
    private void cmd(List<String> args) {
        if (args.isEmpty()) return;
        if (Arrays.asList("run", "up", "tui").contains(args.get(0))) {
            Core.log("use Start/Stop for the daemon");
            return;
        }
        input.setText("");
        boolean mutating = !READ_ONLY.contains(args.get(0)) && args.size() > 1
                || args.get(0).equals("fetch") || args.get(0).equals("add");
        new Thread(() -> {
            boolean restart = mutating && VpnSvc.running;
            if (restart) {
                Core.log("… stopping VPN to apply");
                startService(new Intent(this, VpnSvc.class).setAction(VpnSvc.ACTION_STOP));
                for (int i = 0; i < 100 && VpnSvc.running; i++) sleep(100);
            }
            Core.exec(this, args);
            if (restart) runOnUiThread(this::startVpn);
        }).start();
    }

    private void info() {
        new Thread(() -> {
            Core.exec(this, Arrays.asList("version"));
            Core.exec(this, Arrays.asList("subs"));
            Core.exec(this, Arrays.asList("slot"));
            Core.exec(this, Arrays.asList("whoami"));
        }).start();
    }

    private void startVpn() {
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

    private void stopVpn() {
        startService(new Intent(this, VpnSvc.class).setAction(VpnSvc.ACTION_STOP));
    }

    private LinearLayout row(View... vs) {
        LinearLayout r = new LinearLayout(this);
        for (View v : vs) r.addView(v, new LinearLayout.LayoutParams(0, -2, 1));
        return r;
    }

    private Button btn(String label, View.OnClickListener l) {
        Button b = new Button(this);
        b.setText(label);
        b.setAllCaps(false);
        b.setOnClickListener(l);
        return b;
    }

    private static void sleep(long ms) {
        try { Thread.sleep(ms); } catch (InterruptedException ignored) { }
    }
}
