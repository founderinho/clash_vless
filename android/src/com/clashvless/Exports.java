package com.clashvless;

import android.app.Activity;
import android.graphics.Bitmap;
import android.graphics.Color;

import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.util.function.Consumer;

/** VLESS links, QR codes and pasted-exit parsing, done by the Go binary's hidden `share` command. */
final class Exports {
    private Exports() { }

    /** Runs the request off the UI thread; done gets the response there, or nothing after a logged failure. */
    static void call(Activity activity, JSONObject request, Consumer<JSONObject> done) {
        new Thread(() -> {
            try {
                Process process = new ProcessBuilder(Core.bin(activity), "share").start();
                try (OutputStream in = process.getOutputStream()) {
                    in.write(request.toString().getBytes(StandardCharsets.UTF_8));
                }
                String out = readAll(process.getInputStream()), err = readAll(process.getErrorStream());
                int code = process.waitFor();
                int start = out.indexOf('{');
                if (code != 0 || start < 0) throw new Exception("exit " + code + " " + err.trim());
                JSONObject response = new JSONObject(out.substring(start));
                activity.runOnUiThread(() -> done.accept(response));
            } catch (Exception error) {
                Core.log("Export failed: " + error);
                activity.runOnUiThread(() -> android.widget.Toast.makeText(activity, "Export failed: " + error.getMessage(),
                        android.widget.Toast.LENGTH_SHORT).show());
            }
        }).start();
    }

    private static String readAll(InputStream stream) throws java.io.IOException {
        ByteArrayOutputStream buffer = new ByteArrayOutputStream();
        byte[] chunk = new byte[8192];
        for (int n; (n = stream.read(chunk)) != -1; ) buffer.write(chunk, 0, n);
        return buffer.toString("UTF-8");
    }

    /** Dark modules on white with the standard 4-module quiet zone, scaled by whole pixels so it stays sharp. */
    static Bitmap qr(JSONObject code, int maxPx) {
        if (code == null) return null;
        int size = code.optInt("size");
        String modules = code.optString("modules");
        if (size <= 0 || modules.length() != size * size) return null;
        int side = size + 8, scale = Math.max(1, maxPx / side), px = side * scale;
        int[] pixels = new int[px * px];
        java.util.Arrays.fill(pixels, Color.WHITE);
        for (int y = 0; y < size; y++) {
            for (int x = 0; x < size; x++) {
                if (modules.charAt(y * size + x) != '1') continue;
                for (int dy = 0; dy < scale; dy++) {
                    int row = ((y + 4) * scale + dy) * px + (x + 4) * scale;
                    java.util.Arrays.fill(pixels, row, row + scale, Color.BLACK);
                }
            }
        }
        return Bitmap.createBitmap(pixels, px, px, Bitmap.Config.ARGB_8888);
    }
}
