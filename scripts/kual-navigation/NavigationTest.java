import java.io.*;
import java.util.*;
import com.inkpika.kual.Navigation;
import com.mobileread.ixtab.kindlelauncher.resources.KualEntry;
import com.mobileread.ixtab.kindlelauncher.resources.KualMenu;

public final class NavigationTest {
    private static KualMenu menu() throws Exception {
        KualMenu m = new KualMenu(new BufferedReader(new StringReader("")));
        for (int i = 0; i < 10; i++) m.getLevel(i).clear();
        return m;
    }
    private static KualEntry add(KualMenu menu, int level, String id, String label, boolean submenu) throws Exception {
        String action = submenu ? "^" + (level + 1) + ":" + id + "..aa$" : "/tmp;:";
        KualEntry e = new KualEntry("e", level + ":" + id + "aa", label, action);
        menu.getLevel(level).put(e.id, e);
        return e;
    }
    private static KualEntry[] path(KualMenu menu, String id, String detail) throws Exception {
        KualEntry[] result = new KualEntry[10];
        result[0] = add(menu, 0, id, "InkPika", true);
        result[1] = add(menu, 1, id + "01", "全部书籍 · 0/8 已下载", true);
        result[2] = add(menu, 2, id + "0105", detail, true);
        return result;
    }
    private static void check(boolean value, String message) {
        if (!value) throw new AssertionError(message);
    }
    public static void main(String[] args) throws Exception {
        if (args.length > 0 && args[0].equals("verify")) {
            Class<?> cls = Class.forName("com.mobileread.ixtab.kindlelauncher.KualBooklet", false, NavigationTest.class.getClassLoader());
            cls.getDeclaredMethods(); // Force verification/resolution before installing.
            check(cls.getDeclaredField("inkpikaNavigation") != null, "patch field missing");
        }
        KualMenu old = menu();
        KualEntry[] oldPath = path(old, "01", "[NEW] book.azw3");
        int[] positions = new int[10]; positions[3] = 6;
        Navigation saved = Navigation.capture(oldPath, 3, positions);
        check(saved != null, "sync path not captured");
        positions[3] = 0; // capture must own its copy
        KualMenu fresh = menu();
        KualEntry[] expected = path(fresh, "09", "[OK] book.azw3");
        fresh.getLevel(0).clear();
        expected[0] = add(fresh, 0, "09", "InkPika", true);
        check(Navigation.capture(expected, 3, positions) != null, "project menu not captured");
        fresh.getLevel(1).clear();
        expected[1] = add(fresh, 1, "0901", "全部书籍 · 8/8 已下载", true);
        KualEntry wrongParent = add(fresh, 0, "03", "Other", true);
        add(fresh, 1, "0301", "全部书籍 · 8/8 已下载", true);
        KualEntry[] trail = new KualEntry[10];
        int depth = Navigation.restore(fresh, trail, saved);
        check(depth == 3 && trail[0] == expected[0] && trail[2] == expected[2], "fresh IDs/parent resolution failed");
        for (int i = 0; i < 8; i++) add(fresh, 3, "090105" + (10 + i), "item", false);
        int[] restored = new int[10];
        Navigation.restoreOffsets(fresh, trail, depth, restored, saved, 6);
        check(restored[3] == 6, "page position not preserved");
        fresh.getLevel(3).clear();
        Navigation.restoreOffsets(fresh, trail, depth, restored, saved, 6);
        check(restored[3] == 0, "shorter menu not clamped");
        fresh.getLevel(2).clear();
        check(Navigation.restore(fresh, trail, saved) == 2 && trail[2] == null, "removed submenu must return to sync parent");
        add(fresh, 0, "08", "InkPika", true);
        check(Navigation.restore(fresh, trail, saved) == 0, "ambiguous path must not enter arbitrary extension");
        check(Navigation.restore(menu(), trail, saved) == 0, "removed extension must return to root");
        check(Navigation.capture(oldPath, 0, positions) == null, "root behavior changed");
        Navigation main = Navigation.capture(oldPath, 1, positions);
        check(main != null && Navigation.restore(old, trail, main) == 1, "flat main menu must be retained");
        oldPath[0] = wrongParent;
        check(Navigation.capture(oldPath, 3, positions) == null, "another extension was captured");
        System.out.println("PASS: sync navigation, fresh IDs, parent matching, paging, changed/removed menus, other extensions");
    }
}
