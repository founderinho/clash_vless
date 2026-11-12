package com.clashvless;

import android.content.Context;
import android.view.View;
import android.view.ViewGroup;

/** Lays children left to right and wraps to a new line when the width runs out; rows are centered vertically. */
final class FlowLayout extends ViewGroup {
    private final int gap;

    FlowLayout(Context context, int gapPx) {
        super(context);
        gap = gapPx;
    }

    @Override
    protected void onMeasure(int widthSpec, int heightSpec) {
        int width = MeasureSpec.getMode(widthSpec) == MeasureSpec.UNSPECIFIED ? Integer.MAX_VALUE
                : MeasureSpec.getSize(widthSpec) - getPaddingLeft() - getPaddingRight();
        int x = 0, rowHeight = 0, height = 0, widest = 0;
        for (int i = 0; i < getChildCount(); i++) {
            View child = getChildAt(i);
            if (child.getVisibility() == GONE) continue;
            child.measure(MeasureSpec.makeMeasureSpec(width, MeasureSpec.AT_MOST), MeasureSpec.makeMeasureSpec(0, MeasureSpec.UNSPECIFIED));
            int w = child.getMeasuredWidth(), h = child.getMeasuredHeight();
            if (x > 0 && x + w > width) {
                height += rowHeight + gap;
                x = 0;
                rowHeight = 0;
            }
            x += w + gap;
            widest = Math.max(widest, x - gap);
            rowHeight = Math.max(rowHeight, h);
        }
        height += rowHeight;
        int measuredWidth = MeasureSpec.getMode(widthSpec) == MeasureSpec.EXACTLY ? MeasureSpec.getSize(widthSpec)
                : widest + getPaddingLeft() + getPaddingRight();
        setMeasuredDimension(measuredWidth, resolveSize(height + getPaddingTop() + getPaddingBottom(), heightSpec));
    }

    @Override
    protected void onLayout(boolean changed, int l, int t, int r, int b) {
        int width = r - l - getPaddingLeft() - getPaddingRight();
        int x = 0, y = getPaddingTop(), start = 0;
        while (start < getChildCount()) {
            int end = start, rowWidth = 0, rowHeight = 0;
            for (; end < getChildCount(); end++) {
                View child = getChildAt(end);
                if (child.getVisibility() == GONE) continue;
                int w = child.getMeasuredWidth();
                if (rowWidth > 0 && rowWidth + w > width) break;
                rowWidth += w + gap;
                rowHeight = Math.max(rowHeight, child.getMeasuredHeight());
            }
            x = getPaddingLeft();
            for (int i = start; i < end; i++) {
                View child = getChildAt(i);
                if (child.getVisibility() == GONE) continue;
                int top = y + (rowHeight - child.getMeasuredHeight()) / 2;
                child.layout(x, top, x + child.getMeasuredWidth(), top + child.getMeasuredHeight());
                x += child.getMeasuredWidth() + gap;
            }
            y += rowHeight + gap;
            start = end;
        }
    }
}
