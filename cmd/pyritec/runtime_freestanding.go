package main

const runtimeFreestandingC = `
#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>

typedef struct {
    long *items;
    size_t len;
} PyriteList;

typedef enum {
    PYRITE_ANY_INT,
    PYRITE_ANY_FLOAT,
    PYRITE_ANY_BOOL,
    PYRITE_ANY_STRING
} PyriteAnyKind;

typedef struct {
    PyriteAnyKind kind;
    union {
        long i;
        double f;
        int b;
        char *s;
    } as;
} PyriteAny;

typedef struct {
    PyriteAny *items;
    size_t len;
} PyriteAnyList;

typedef struct {
    char *name;
    PyriteAny value;
} PyriteClassField;

typedef struct {
    char *class_name;
    PyriteClassField *fields;
    size_t len;
    size_t cap;
} PyriteClassObject;

typedef struct PyriteAllocation {
    void *ptr;
    size_t bytes;
    struct PyriteAllocation *next;
} PyriteAllocation;

typedef struct PyriteObject {
    struct PyriteObject *parent;
    char *name;
    int has_name;
    char *kind;
    int has_kind;
    long score;
    int has_score;
    int active;
    int has_active;
} PyriteObject;

typedef struct { int unused; } PyriteMux;
typedef struct { int fd; } PyriteSocket;
typedef struct { int fd; } PyriteListener;
typedef struct PyriteFreestandingFile FILE;

__attribute__((weak)) void pyrite_kernel_putchar(int ch) {
    (void)ch;
}

__attribute__((weak, noreturn)) void pyrite_kernel_hang(void) {
    for (;;) {
#if defined(__x86_64__) || defined(__i386__)
        __asm__ volatile("hlt");
#endif
    }
}

__attribute__((weak)) long pyrite_kernel_cls(void) {
    pyrite_kernel_putchar(27);
    pyrite_kernel_putchar('[');
    pyrite_kernel_putchar('2');
    pyrite_kernel_putchar('J');
    pyrite_kernel_putchar(27);
    pyrite_kernel_putchar('[');
    pyrite_kernel_putchar('H');
    return 0;
}

__attribute__((weak)) char *pyrite_kernel_input(const char *prompt) {
    (void)prompt;
    static char empty[1] = {0};
    return empty;
}

__attribute__((weak)) long pyrite_keyboard_read_scancode(void) {
    return 0;
}

__attribute__((weak)) long pyrite_keyboard_clear(void) {
    return 0;
}

__attribute__((weak)) long pyrite_keyboard_append_ascii(long ascii) {
    (void)ascii;
    return 0;
}

__attribute__((weak)) long pyrite_keyboard_backspace(void) {
    return 0;
}

__attribute__((weak)) char *pyrite_keyboard_value(void) {
    static char empty[1] = {0};
    return empty;
}

static unsigned char pyrite_heap[64 * 1024];
static size_t pyrite_heap_used = 0;
static unsigned char pyrite_temp_arena[8 * 1024];
static size_t pyrite_temp_used = 0;
static char pyrite_last_error[128] = "";

static size_t pyrite_strlen(const char *s) {
    size_t n = 0;
    if (!s) return 0;
    while (s[n]) n++;
    return n;
}

static long pyrite_string_len(const char *s) {
    return (long)pyrite_strlen(s);
}

int strcmp(const char *left, const char *right) {
    unsigned char l;
    unsigned char r;
    if (!left) left = "";
    if (!right) right = "";
    do {
        l = (unsigned char)*left++;
        r = (unsigned char)*right++;
        if (l != r) return (int)l - (int)r;
    } while (l);
    return 0;
}

static void pyrite_memcpy(void *dst, const void *src, size_t n) {
    unsigned char *d = (unsigned char *)dst;
    const unsigned char *s = (const unsigned char *)src;
    for (size_t i = 0; i < n; i++) d[i] = s[i];
}

static void pyrite_memset(void *dst, int value, size_t n) {
    unsigned char *d = (unsigned char *)dst;
    for (size_t i = 0; i < n; i++) d[i] = (unsigned char)value;
}

static void *pyrite_malloc(size_t bytes) {
    if (bytes == 0) bytes = 1;
    bytes = (bytes + 7) & ~(size_t)7;
    if (pyrite_heap_used + bytes > sizeof(pyrite_heap)) return NULL;
    void *ptr = pyrite_heap + pyrite_heap_used;
    pyrite_heap_used += bytes;
    pyrite_memset(ptr, 0, bytes);
    return ptr;
}

static void *pyrite_calloc(size_t count, size_t bytes) {
    return pyrite_malloc(count * bytes);
}

static void *pyrite_temp_alloc(size_t bytes) {
    if (bytes == 0) bytes = 1;
    bytes = (bytes + 7) & ~(size_t)7;
    if (pyrite_temp_used + bytes > sizeof(pyrite_temp_arena)) return NULL;
    void *ptr = pyrite_temp_arena + pyrite_temp_used;
    pyrite_temp_used += bytes;
    pyrite_memset(ptr, 0, bytes);
    return ptr;
}

static void pyrite_temp_reset(void) {
    pyrite_temp_used = 0;
}

static void pyrite_release(void *ptr) {
    (void)ptr;
}

static PyriteAllocation *pyrite_checkpoint(void) {
    return NULL;
}

static void pyrite_release_since(PyriteAllocation *checkpoint, void *keep) {
    (void)checkpoint;
    (void)keep;
}

static void pyrite_release_since_any_list(PyriteAllocation *checkpoint, PyriteAnyList *keep) {
    (void)checkpoint;
    (void)keep;
}

static void pyrite_release_since_class_object(PyriteAllocation *checkpoint, PyriteClassObject *keep) {
    (void)checkpoint;
    (void)keep;
}

static char *pyrite_promote_string(const char *s) {
    if (!s) s = "";
    size_t n = pyrite_strlen(s);
    char *out = pyrite_malloc(n + 1);
    if (!out) return "";
    pyrite_memcpy(out, s, n + 1);
    return out;
}

static void pyrite_assign_string(char **slot, const char *s) {
    if (!slot) return;
    *slot = pyrite_promote_string(s);
}

static char *pyrite_last_error_or(const char *fallback) {
    return pyrite_last_error[0] ? pyrite_last_error : (char *)(fallback ? fallback : "runtime error");
}

static char *pyrite_last_error_value(void) {
    return pyrite_last_error_or("");
}

static void pyrite_write_char(char ch) {
    pyrite_kernel_putchar((int)ch);
}

static long pyrite_kernel_print(const char *s) {
    long n = 0;
    if (!s) s = "";
    while (*s) {
        pyrite_write_char(*s++);
        n++;
    }
    return n;
}

static long pyrite_kernel_println(const char *s) {
    long n = pyrite_kernel_print(s);
    pyrite_write_char('\n');
    return n + 1;
}

static void pyrite_print_str(const char *s) {
    (void)pyrite_kernel_println(s);
}

static void pyrite_print_unsigned(unsigned long value) {
    char buf[32];
    size_t used = 0;
    if (value == 0) {
        pyrite_write_char('0');
        return;
    }
    while (value && used < sizeof(buf)) {
        buf[used++] = (char)('0' + (value % 10));
        value /= 10;
    }
    while (used) pyrite_write_char(buf[--used]);
}

static void pyrite_print_int(long value) {
    if (value < 0) {
        pyrite_write_char('-');
        pyrite_print_unsigned((unsigned long)(-value));
    } else {
        pyrite_print_unsigned((unsigned long)value);
    }
    pyrite_write_char('\n');
}

static void pyrite_print_float(double value) {
    (void)value;
    pyrite_print_str("<float>");
}

static void pyrite_append_char(char **cursor, char *end, char ch) {
    if (*cursor + 1 >= end) return;
    **cursor = ch;
    (*cursor)++;
    **cursor = '\0';
}

static void pyrite_append_str(char **cursor, char *end, const char *s) {
    if (!s) s = "";
    while (*s) pyrite_append_char(cursor, end, *s++);
}

static void pyrite_append_long(char **cursor, char *end, long value) {
    char tmp[32];
    size_t used = 0;
    unsigned long v;
    if (value < 0) {
        pyrite_append_char(cursor, end, '-');
        v = (unsigned long)(-value);
    } else {
        v = (unsigned long)value;
    }
    if (v == 0) {
        pyrite_append_char(cursor, end, '0');
        return;
    }
    while (v && used < sizeof(tmp)) {
        tmp[used++] = (char)('0' + (v % 10));
        v /= 10;
    }
    while (used) pyrite_append_char(cursor, end, tmp[--used]);
}

static char *pyrite_fmt(const char *fmt, ...) {
    char *buf = pyrite_temp_alloc(512);
    char *cursor = buf;
    char *end = buf ? buf + 512 : NULL;
    if (!buf) return "";
    va_list ap;
    va_start(ap, fmt);
    for (const char *p = fmt; p && *p; p++) {
        if (*p != '%') {
            pyrite_append_char(&cursor, end, *p);
            continue;
        }
        p++;
        if (*p == 's') {
            pyrite_append_str(&cursor, end, va_arg(ap, const char *));
        } else if (*p == 'l' && p[1] == 'd') {
            p++;
            pyrite_append_long(&cursor, end, va_arg(ap, long));
        } else if (*p == 'd') {
            pyrite_append_long(&cursor, end, (long)va_arg(ap, int));
        } else if (*p == 'g') {
            (void)va_arg(ap, double);
            pyrite_append_str(&cursor, end, "<float>");
        } else if (*p == '%') {
            pyrite_append_char(&cursor, end, '%');
        }
    }
    va_end(ap);
    return buf;
}

static char *pyrite_any_string(PyriteAny value) {
    switch (value.kind) {
    case PYRITE_ANY_INT:
        return pyrite_fmt("%ld", value.as.i);
    case PYRITE_ANY_FLOAT:
        return "<float>";
    case PYRITE_ANY_BOOL:
        return value.as.b ? "true" : "false";
    case PYRITE_ANY_STRING:
        return value.as.s ? value.as.s : "";
    default:
        return "";
    }
}

static void pyrite_print_any(PyriteAny value) {
    pyrite_print_str(pyrite_any_string(value));
}

static void pyrite_release_any(PyriteAny *value) {
    (void)value;
}

static void pyrite_release_any_list(PyriteAnyList *list) {
    (void)list;
}

static PyriteList pyrite_list_int_new(long *items, long len) {
    PyriteList list = {0};
    if (len <= 0) return list;
    list.items = pyrite_malloc(sizeof(long) * (size_t)len);
    if (!list.items) return list;
    list.len = (size_t)len;
    pyrite_memcpy(list.items, items, sizeof(long) * (size_t)len);
    return list;
}

static PyriteAnyList pyrite_list_any_new(PyriteAny *items, long len) {
    PyriteAnyList list = {0};
    if (len <= 0) return list;
    list.items = pyrite_malloc(sizeof(PyriteAny) * (size_t)len);
    if (!list.items) return list;
    list.len = (size_t)len;
    pyrite_memcpy(list.items, items, sizeof(PyriteAny) * (size_t)len);
    return list;
}

static PyriteAnyList pyrite_list_int_to_any(PyriteList list) {
    PyriteAnyList out = {0};
    if (list.len == 0) return out;
    out.items = pyrite_malloc(sizeof(PyriteAny) * list.len);
    if (!out.items) return out;
    out.len = list.len;
    for (size_t i = 0; i < list.len; i++) {
        out.items[i] = (PyriteAny){.kind=PYRITE_ANY_INT, .as.i=list.items[i]};
    }
    return out;
}

static char *pyrite_list_int_string(PyriteList *list) {
    char *buf = pyrite_temp_alloc(512);
    char *cursor = buf;
    char *end = buf ? buf + 512 : NULL;
    if (!buf) return "";
    pyrite_append_char(&cursor, end, '[');
    for (size_t i = 0; list && i < list->len; i++) {
        if (i) pyrite_append_str(&cursor, end, ", ");
        pyrite_append_long(&cursor, end, list->items[i]);
    }
    pyrite_append_char(&cursor, end, ']');
    return buf;
}

static char *pyrite_list_any_string(PyriteAnyList *list) {
    char *buf = pyrite_temp_alloc(512);
    char *cursor = buf;
    char *end = buf ? buf + 512 : NULL;
    if (!buf) return "";
    pyrite_append_char(&cursor, end, '[');
    for (size_t i = 0; list && i < list->len; i++) {
        if (i) pyrite_append_str(&cursor, end, ", ");
        pyrite_append_str(&cursor, end, pyrite_any_string(list->items[i]));
    }
    pyrite_append_char(&cursor, end, ']');
    return buf;
}

static void pyrite_join_routines(void) {}

static size_t pyrite_defer_memory_cleanup(void) {
    pyrite_temp_reset();
    return 0;
}

static long pyrite_kernel_panic(const char *message) {
    (void)pyrite_kernel_print("panic: ");
    (void)pyrite_kernel_println(message ? message : "");
    pyrite_kernel_hang();
}

static long pyrite_kernel_halt(void) {
    pyrite_kernel_hang();
}

static long pyrite_kernel_write_port(long port, long value) {
#if defined(__x86_64__) || defined(__i386__)
    __asm__ volatile("outb %0, %1" : : "a"((uint8_t)value), "Nd"((uint16_t)port));
#else
    (void)port;
    (void)value;
#endif
    return 0;
}

static long pyrite_kernel_read_port(long port) {
    uint8_t value = 0;
#if defined(__x86_64__) || defined(__i386__)
    __asm__ volatile("inb %1, %0" : "=a"(value) : "Nd"((uint16_t)port));
#else
    (void)port;
#endif
    return (long)value;
}

static long pyrite_kernel_outb(long port, long value) {
    return pyrite_kernel_write_port(port, value);
}

static long pyrite_kernel_inb(long port) {
    return pyrite_kernel_read_port(port);
}
`
