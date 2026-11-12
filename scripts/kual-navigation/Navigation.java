package com.inkpika.kual;

import java.util.Iterator;
import com.mobileread.ixtab.kindlelauncher.resources.KualEntry;
import com.mobileread.ixtab.kindlelauncher.resources.KualMenu;

/** Retains InkPika's location across KUAL cache reloads. */
public final class Navigation {
    private final String[] labels;
    private final int[] positions;

    private Navigation(String[] labels, int[] positions) {
        this.labels = labels;
        this.positions = positions;
    }

    public static Navigation capture(KualEntry[] trail, int depth, int[] offsets) {
        if (depth < 1 || depth >= trail.length || trail[0] == null
                || !"InkPika".equals(trail[0].getBareLabel())) {
            return null;
        }
        String[] labels = new String[depth];
        for (int i = 0; i < depth; i++) {
            if (trail[i] == null || !trail[i].isSubmenu) return null;
            labels[i] = identity(trail[i].getBareLabel(), i);
        }
        return new Navigation(labels, (int[]) offsets.clone());
    }

    // Counts and status prefixes change after a scan/download; the section or
    // book identity does not. Ambiguous truncated titles still fall back safely.
    private static String identity(String label, int level) {
        if (level == 0 && "InkPika".equals(label)) return "InkPika";
        if (level == 1) {
            if (label.startsWith("全部书籍 · ")) return "全部书籍";
            if (label.startsWith("查看状态 · ")) return "查看状态";
        }
        if (level == 2) {
            String[] codes = {"[OK] ", "[NEW] ", "[UPD] ", "[ERR] ", "[DL] "};
            for (int i = 0; i < codes.length; i++) {
                if (label.startsWith(codes[i])) return label.substring(codes[i].length());
            }
        }
        return label;
    }

    // IDs can change when extensions are added or removed. Resolve the current
    // entries by label AND parent, rather than retaining stale objects/IDs.
    public static int restore(KualMenu menu, KualEntry[] trail, Navigation saved) {
        for (int i = 0; i < trail.length; i++) trail[i] = null;
        int depth = 0;
        while (depth < saved.labels.length && depth + 1 < trail.length) {
            KualEntry found = null;
            Iterator entries = menu.getLevel(depth).values().iterator();
            while (entries.hasNext()) {
                KualEntry entry = (KualEntry) entries.next();
                if (entry.isSubmenu && saved.labels[depth].equals(identity(entry.getBareLabel(), depth))
                        && entry.getGoToLevel() == depth + 1
                        && (depth == 0 || entry.isLinkedUnder(trail[depth - 1].getParentLink()))) {
                    // Ambiguous labels are unsafe: fall back to the parent.
                    if (found != null) { found = null; break; }
                    found = entry;
                }
            }
            if (found == null) break;
            trail[depth++] = found;
        }
        return depth;
    }

    // A shorter updated submenu must not leave its viewport past the last item.
    public static void restoreOffsets(KualMenu menu, KualEntry[] trail, int depth,
            int[] offsets, Navigation saved, int pageSize) {
        if (saved == null || pageSize <= 0) return;
        for (int level = 0; level <= depth && level < saved.positions.length; level++) {
            int count = 1; // KUAL's injected Home/Quit button.
            Iterator entries = menu.getLevel(level).values().iterator();
            while (entries.hasNext()) {
                KualEntry entry = (KualEntry) entries.next();
                if (level == 0 || entry.isLinkedUnder(trail[level - 1].getParentLink())) count++;
            }
            int lastPage = ((count - 1) / pageSize) * pageSize;
            int requested = Math.max(0, saved.positions[level]);
            offsets[level] = Math.min((requested / pageSize) * pageSize, lastPage);
        }
    }
}
