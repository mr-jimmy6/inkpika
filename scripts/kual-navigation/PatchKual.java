import java.io.*;
import java.nio.file.*;
import java.security.MessageDigest;
import java.util.*;
import java.util.zip.*;
import org.objectweb.asm.*;

/** Offline patcher: modifies the user's own JAR, never downloads/replaces KUAL. */
public final class PatchKual implements Opcodes {
    private static final String OWNER = "com/mobileread/ixtab/kindlelauncher/KualBooklet";
    private static final String RESOURCE = "com/mobileread/ixtab/kindlelauncher/resources/";
    private static final String NAV = "com/inkpika/kual/Navigation";
    private static final String NAV_TYPE = "L" + NAV + ";";
    private static final String TRAIL = "[L" + RESOURCE + "KualEntry;";
    private static final String MENU = "L" + RESOURCE + "KualMenu;";
    private static final String FIELD = "inkpikaNavigation";
    // Exact tested PW2 KUAL v2.7.32-g74c69f9; refuse unknown bytecode.
    private static final String EXPECTED_JAR = "c13fc2bbb2890530a5c6374b879b2049a6136028713c37794d9f40093ea8dae3";

    private static void get(MethodVisitor mv, String name, String type) {
        mv.visitVarInsn(ALOAD, 0);
        mv.visitFieldInsn(GETFIELD, OWNER, name, type);
    }
    public static byte[] patch(byte[] input) {
        final int[] hooks = new int[5];
        ClassReader reader = new ClassReader(input);
        ClassWriter writer = new ClassWriter(ClassWriter.COMPUTE_MAXS);
        reader.accept(new ClassVisitor(ASM9, writer) {
            public void visit(int version, int access, String name, String signature, String parent, String[] interfaces) {
                if (version != V1_4 || !OWNER.equals(name)) throw new IllegalArgumentException("Unexpected KUAL class");
                super.visit(version, access, name, signature, parent, interfaces);
            }
            public void visitEnd() {
                super.visitField(ACC_PRIVATE, FIELD, NAV_TYPE, null, null).visitEnd();
                super.visitEnd();
            }
            public MethodVisitor visitMethod(int access, final String name, final String desc, String signature, String[] exceptions) {
                MethodVisitor base = super.visitMethod(access, name, desc, signature, exceptions);
                return new MethodVisitor(ASM9, base) {
                    private boolean firstClear = true;
                    public void visitCode() {
                        super.visitCode();
                        if (name.equals("readParser") && desc.equals("(Ljava/io/BufferedReader;)V")) {
                            hooks[0]++;
                            Label done = new Label();
                            get(mv, FIELD, NAV_TYPE);
                            mv.visitJumpInsn(IFNONNULL, done);
                            mv.visitVarInsn(ALOAD, 0);
                            get(mv, "keTrail", TRAIL);
                            get(mv, "depth", "I");
                            get(mv, "offset", "[I");
                            mv.visitMethodInsn(INVOKESTATIC, NAV, "capture", "(" + TRAIL + "I[I)" + NAV_TYPE, false);
                            mv.visitFieldInsn(PUTFIELD, OWNER, FIELD, NAV_TYPE);
                            mv.visitLabel(done);
                        } else if (name.equals("setStatus") && desc.equals("(Ljava/lang/String;)V")) {
                            hooks[4]++;
                            Label done = new Label();
                            get(mv, FIELD, NAV_TYPE);
                            mv.visitJumpInsn(IFNULL, done);
                            mv.visitLdcInsn("New menu loaded. Please go to top.");
                            mv.visitVarInsn(ALOAD, 1);
                            mv.visitMethodInsn(INVOKEVIRTUAL, "java/lang/String", "equals", "(Ljava/lang/Object;)Z", false);
                            mv.visitJumpInsn(IFEQ, done);
                            mv.visitLdcInsn("Menu updated.");
                            mv.visitVarInsn(ASTORE, 1);
                            mv.visitLabel(done);
                        } else if (name.equals("actionPerformed") && desc.equals("(Ljava/awt/event/ActionEvent;)V")) {
                            hooks[1]++;
                            // A new user action supersedes an older refresh's saved location.
                            mv.visitVarInsn(ALOAD, 0);
                            mv.visitInsn(ACONST_NULL);
                            mv.visitFieldInsn(PUTFIELD, OWNER, FIELD, NAV_TYPE);
                        } else if (name.equals("updateDisplayedLaunchers") && desc.equals("(IZLjava/awt/Component;)V")) {
                            hooks[2]++;
                            Label done = new Label();
                            mv.visitVarInsn(ILOAD, 2); // resetViewport
                            mv.visitJumpInsn(IFEQ, done);
                            get(mv, FIELD, NAV_TYPE);
                            mv.visitJumpInsn(IFNULL, done);
                            mv.visitVarInsn(ALOAD, 0);
                            get(mv, "kualMenu", MENU);
                            get(mv, "keTrail", TRAIL);
                            get(mv, FIELD, NAV_TYPE);
                            mv.visitMethodInsn(INVOKESTATIC, NAV, "restore", "(" + MENU + TRAIL + NAV_TYPE + ")I", false);
                            mv.visitInsn(DUP);
                            mv.visitVarInsn(ISTORE, 1); // override requested level
                            mv.visitFieldInsn(PUTFIELD, OWNER, "depth", "I");
                            mv.visitLabel(done);
                        }
                    }
                    public void visitMethodInsn(int opcode, String owner, String called, String descriptor, boolean itf) {
                        super.visitMethodInsn(opcode, owner, called, descriptor, itf);
                        if (name.equals("updateDisplayedLaunchers") && firstClear
                                && owner.equals("java/util/ArrayList") && called.equals("clear")) {
                            firstClear = false;
                            hooks[3]++;
                            // First clear is inside resetViewport, after zeroing offsets.
                            get(mv, "kualMenu", MENU);
                            get(mv, "keTrail", TRAIL);
                            get(mv, "depth", "I");
                            get(mv, "offset", "[I");
                            get(mv, FIELD, NAV_TYPE);
                            mv.visitVarInsn(ALOAD, 0);
                            mv.visitMethodInsn(INVOKESPECIAL, OWNER, "getPageSize", "()I", false);
                            mv.visitMethodInsn(INVOKESTATIC, NAV, "restoreOffsets", "(" + MENU + TRAIL + "I[I" + NAV_TYPE + "I)V", false);
                        }
                    }
                };
            }
        }, 0);
        if (!Arrays.equals(hooks, new int[]{1, 1, 1, 1, 1})) throw new IllegalArgumentException("KUAL hook mismatch");
        return writer.toByteArray();
    }
    private static byte[] read(InputStream in) throws IOException {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        byte[] b = new byte[8192]; int n;
        while ((n = in.read(b)) != -1) out.write(b, 0, n);
        return out.toByteArray();
    }
    public static String hash(byte[] bytes) throws Exception {
        StringBuilder result = new StringBuilder();
        for (byte b : MessageDigest.getInstance("SHA-256").digest(bytes)) result.append(String.format("%02x", b & 255));
        return result.toString();
    }
    public static void main(String[] args) throws Exception {
        if (args.length != 3) throw new IllegalArgumentException("input.jar output.jar Navigation.class");
        if (!EXPECTED_JAR.equals(hash(Files.readAllBytes(Paths.get(args[0]))))) throw new IllegalArgumentException("Unsupported KUAL JAR; no output written");
        try (ZipFile input = new ZipFile(args[0]); ZipOutputStream output = new ZipOutputStream(new FileOutputStream(args[1]))) {
            Enumeration<? extends ZipEntry> entries = input.entries();
            while (entries.hasMoreElements()) {
                ZipEntry entry = entries.nextElement();
                byte[] bytes;
                try (InputStream in = input.getInputStream(entry)) { bytes = read(in); }
                if (entry.getName().equals(OWNER + ".class")) bytes = patch(bytes);
                ZipEntry copy = new ZipEntry(entry.getName()); copy.setTime(entry.getTime());
                output.putNextEntry(copy); output.write(bytes); output.closeEntry();
            }
            ZipEntry helper = new ZipEntry(NAV + ".class"); helper.setTime(0);
            output.putNextEntry(helper); output.write(Files.readAllBytes(Paths.get(args[2]))); output.closeEntry();
        }
        System.out.println("Patched KUAL SHA256: " + hash(Files.readAllBytes(Paths.get(args[1]))));
    }
}
