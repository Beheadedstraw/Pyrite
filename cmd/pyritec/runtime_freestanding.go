package main

const runtimeFreestandingC = `
#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>

typedef struct {
    long *items;
    size_t len;
    size_t cap;
} PyriteList;

typedef struct {
    unsigned char *items;
    size_t len;
    size_t cap;
} PyriteBytes;

typedef struct {
    char *items;
    size_t len;
    size_t cap;
} PyriteStringBuilder;

typedef PyriteBytes PyriteBytesBuilder;
typedef struct PyriteClassObject PyriteClassObject;

typedef enum {
    PYRITE_ANY_INT,
    PYRITE_ANY_FLOAT,
    PYRITE_ANY_BOOL,
    PYRITE_ANY_STRING,
    PYRITE_ANY_BYTES,
    PYRITE_ANY_CLASS
} PyriteAnyKind;

typedef struct {
    PyriteAnyKind kind;
    union {
        long i;
        double f;
        int b;
        char *s;
        PyriteBytes bytes;
        PyriteClassObject *obj;
    } as;
} PyriteAny;

typedef struct {
    PyriteAny *items;
    size_t len;
    size_t cap;
} PyriteAnyList;

typedef struct {
    char *key;
    PyriteAny value;
} PyriteDictEntry;

typedef struct {
    PyriteDictEntry *entries;
    size_t len;
    size_t cap;
} PyriteDict;

typedef struct {
    char **items;
    size_t len;
    size_t cap;
} PyriteSet;

static PyriteAny pyrite_any_clone(PyriteAny value);
static void pyrite_release_any(PyriteAny *value);
static char *pyrite_any_string(PyriteAny value);
static char *pyrite_promote_string(const char *s);

typedef struct {
    char *name;
    PyriteAny value;
} PyriteClassField;

struct PyriteClassObject {
    char *class_name;
    PyriteClassField *fields;
    size_t len;
    size_t cap;
};

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

long artemis_setup_user_mode(long kernel_cr3);
long artemis_enter_user(long space, long entry, long stack);

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

__attribute__((weak)) long pyrite_kernel_color(long value) {
    (void)value;
    return 0;
}

__attribute__((weak)) long pyrite_keyboard_read_scancode(void) {
    return 0;
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

static int pyrite_ascii_space(unsigned char ch) {
    return ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' || ch == '\v' || ch == '\f';
}

static int pyrite_ascii_digit(unsigned char ch) {
    return ch >= '0' && ch <= '9';
}

static int pyrite_ascii_alpha(unsigned char ch) {
    return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z');
}

static char pyrite_ascii_upper(unsigned char ch) {
    return (ch >= 'a' && ch <= 'z') ? (char)(ch - 32) : (char)ch;
}

static char pyrite_ascii_lower(unsigned char ch) {
    return (ch >= 'A' && ch <= 'Z') ? (char)(ch + 32) : (char)ch;
}

static long pyrite_string_len(const char *s) {
    return (long)pyrite_strlen(s);
}

static void *pyrite_malloc(size_t bytes);
static void pyrite_memcpy(void *dst, const void *src, size_t n);

static char *pyrite_string_concat(const char *left, const char *right) {
    if (!left) left = "";
    if (!right) right = "";
    size_t left_len = pyrite_strlen(left);
    size_t right_len = pyrite_strlen(right);
    char *out = pyrite_malloc(left_len + right_len + 1);
    if (!out) return "";
    pyrite_memcpy(out, left, left_len);
    pyrite_memcpy(out + left_len, right, right_len);
    out[left_len + right_len] = '\0';
    return out;
}

static char *pyrite_chr(long value) {
    char *out = pyrite_malloc(2);
    if (!out) return "";
    if (value < 0 || value > 255) value = 0;
    out[0] = (char)value;
    out[1] = '\0';
    return out;
}

static char *pyrite_string_slice(const char *s, long start, long end) {
    if (!s) s = "";
    long len = (long)pyrite_strlen(s);
    if (start < 0) start = 0;
    if (end < start) end = start;
    if (start > len) start = len;
    if (end > len) end = len;
    long out_len = end - start;
    char *out = pyrite_malloc((size_t)out_len + 1);
    if (!out) return "";
    for (long i = 0; i < out_len; i++) out[i] = s[start + i];
    out[out_len] = '\0';
    return out;
}

static char *pyrite_string_at(const char *s, long index) {
    if (!s || index < 0 || index >= (long)pyrite_strlen(s)) return pyrite_promote_string("");
    char *out = pyrite_malloc(2);
    if (!out) return "";
    out[0] = s[index];
    out[1] = '\0';
    return out;
}

static long pyrite_string_byte_at(const char *s, long index) {
    if (!s || index < 0 || index >= (long)pyrite_strlen(s)) return 0;
    return (long)(unsigned char)s[index];
}

static char *pyrite_string_lstrip(const char *s) {
    if (!s) s = "";
    size_t start = 0;
    size_t len = pyrite_strlen(s);
    while (start < len && pyrite_ascii_space((unsigned char)s[start])) start++;
    return pyrite_string_slice(s, (long)start, (long)len);
}

static char *pyrite_string_rstrip(const char *s) {
    if (!s) s = "";
    size_t end = pyrite_strlen(s);
    while (end > 0 && pyrite_ascii_space((unsigned char)s[end - 1])) end--;
    return pyrite_string_slice(s, 0, (long)end);
}

static char *pyrite_string_strip(const char *s) {
    if (!s) s = "";
    size_t start = 0;
    size_t end = pyrite_strlen(s);
    while (start < end && pyrite_ascii_space((unsigned char)s[start])) start++;
    while (end > start && pyrite_ascii_space((unsigned char)s[end - 1])) end--;
    return pyrite_string_slice(s, (long)start, (long)end);
}

static char *pyrite_string_upper(const char *s) {
    if (!s) s = "";
    size_t len = pyrite_strlen(s);
    char *out = pyrite_malloc(len + 1);
    if (!out) return "";
    for (size_t i = 0; i < len; i++) out[i] = pyrite_ascii_upper((unsigned char)s[i]);
    out[len] = '\0';
    return out;
}

static char *pyrite_string_lower(const char *s) {
    if (!s) s = "";
    size_t len = pyrite_strlen(s);
    char *out = pyrite_malloc(len + 1);
    if (!out) return "";
    for (size_t i = 0; i < len; i++) out[i] = pyrite_ascii_lower((unsigned char)s[i]);
    out[len] = '\0';
    return out;
}

static int pyrite_string_startswith(const char *s, const char *prefix) {
    if (!s) s = "";
    if (!prefix) prefix = "";
    while (*prefix) {
        if (*s != *prefix) return 0;
        s++;
        prefix++;
    }
    return 1;
}

static long pyrite_string_find(const char *s, const char *needle) {
    if (!s) s = "";
    if (!needle || !*needle) return 0;
    size_t needle_len = pyrite_strlen(needle);
    for (size_t i = 0; s[i]; i++) {
        size_t j = 0;
        while (j < needle_len && s[i + j] && s[i + j] == needle[j]) j++;
        if (j == needle_len) return (long)i;
    }
    return -1;
}

static int pyrite_string_contains(const char *s, const char *needle) {
    return pyrite_string_find(s, needle) >= 0;
}

static int pyrite_string_endswith(const char *s, const char *suffix) {
    if (!s) s = "";
    if (!suffix) suffix = "";
    size_t len = pyrite_strlen(s);
    size_t suffix_len = pyrite_strlen(suffix);
    if (suffix_len > len) return 0;
    for (size_t i = 0; i < suffix_len; i++) {
        if (s[len - suffix_len + i] != suffix[i]) return 0;
    }
    return 1;
}

static char *pyrite_string_replace(const char *s, const char *old, const char *replacement) {
    if (!s) s = "";
    if (!old) old = "";
    if (!replacement) replacement = "";
    size_t old_len = pyrite_strlen(old);
    if (old_len == 0) return pyrite_promote_string(s);
    size_t repl_len = pyrite_strlen(replacement);
    size_t count = 0;
    const char *p = s;
    while (*p) {
        size_t j = 0;
        while (j < old_len && p[j] && p[j] == old[j]) j++;
        if (j == old_len) {
            count++;
            p += old_len;
        } else {
            p++;
        }
    }
    size_t len = pyrite_strlen(s);
    size_t out_len = repl_len >= old_len ? len + count * (repl_len - old_len) : len - count * (old_len - repl_len);
    char *out = pyrite_malloc(out_len + 1);
    if (!out) return "";
    char *dst = out;
    p = s;
    while (*p) {
        size_t j = 0;
        while (j < old_len && p[j] && p[j] == old[j]) j++;
        if (j == old_len) {
            pyrite_memcpy(dst, replacement, repl_len);
            dst += repl_len;
            p += old_len;
        } else {
            *dst++ = *p++;
        }
    }
    *dst = '\0';
    return out;
}

static long pyrite_string_to_int(const char *s) {
    if (!s) return 0;
    while (pyrite_ascii_space((unsigned char)*s)) s++;
    int sign = 1;
    if (*s == '-') {
        sign = -1;
        s++;
    } else if (*s == '+') {
        s++;
    }
    long value = 0;
    while (pyrite_ascii_digit((unsigned char)*s)) {
        value = value * 10 + (long)(*s - '0');
        s++;
    }
    return value * sign;
}

static int pyrite_string_is_digit(const char *s) {
    if (!s || !*s) return 0;
    for (; *s; s++) if (!pyrite_ascii_digit((unsigned char)*s)) return 0;
    return 1;
}

static int pyrite_string_is_alpha(const char *s) {
    if (!s || !*s) return 0;
    for (; *s; s++) if (!pyrite_ascii_alpha((unsigned char)*s)) return 0;
    return 1;
}

static int pyrite_string_is_alnum(const char *s) {
    if (!s || !*s) return 0;
    for (; *s; s++) {
        unsigned char ch = (unsigned char)*s;
        if (!pyrite_ascii_alpha(ch) && !pyrite_ascii_digit(ch)) return 0;
    }
    return 1;
}

static int pyrite_string_is_space(const char *s) {
    if (!s || !*s) return 0;
    for (; *s; s++) if (!pyrite_ascii_space((unsigned char)*s)) return 0;
    return 1;
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

static PyriteBytes pyrite_bytes_from_data(const unsigned char *data, long len) {
    PyriteBytes out = {0};
    if (len <= 0) return out;
    out.items = pyrite_malloc((size_t)len);
    if (!out.items) return out;
    out.len = (size_t)len;
    pyrite_memcpy(out.items, data, (size_t)len);
    return out;
}

static PyriteBytes pyrite_bytes_copy(PyriteBytes value) {
    return pyrite_bytes_from_data(value.items, (long)value.len);
}

static PyriteBytes pyrite_bytes_from_list(PyriteList list) {
    PyriteBytes out = {0};
    if (list.len == 0) return out;
    out.items = pyrite_malloc(list.len);
    if (!out.items) return out;
    out.len = list.len;
    for (size_t i = 0; i < list.len; i++) {
        long value = list.items[i];
        if (value < 0) value = 0;
        if (value > 255) value = 255;
        out.items[i] = (unsigned char)value;
    }
    return out;
}

static long pyrite_bytes_len(PyriteBytes value) {
    return (long)value.len;
}

static long pyrite_bytes_get(PyriteBytes value, long index) {
    if (index < 0 || (size_t)index >= value.len || !value.items) return 0;
    return (long)value.items[index];
}

static PyriteBytes pyrite_bytes_slice(PyriteBytes value, long start, long end) {
    if (start < 0) start = 0;
    if (end < start) end = start;
    if ((size_t)start > value.len) start = (long)value.len;
    if ((size_t)end > value.len) end = (long)value.len;
    return pyrite_bytes_from_data(value.items + start, end - start);
}

static PyriteBytes pyrite_bytes_push(PyriteBytes value, long byte_value) {
    PyriteBytes out = {0};
    out.items = pyrite_malloc(value.len + 1);
    if (!out.items) return out;
    out.len = value.len + 1;
    if (value.items && value.len) pyrite_memcpy(out.items, value.items, value.len);
    if (byte_value < 0) byte_value = 0;
    if (byte_value > 255) byte_value = 255;
    out.items[value.len] = (unsigned char)byte_value;
    return out;
}

static PyriteBytes pyrite_bytes_concat(PyriteBytes left, PyriteBytes right) {
    PyriteBytes out = {0};
    out.len = left.len + right.len;
    if (out.len == 0) return out;
    out.items = pyrite_malloc(out.len);
    if (!out.items) {
        out.len = 0;
        return out;
    }
    if (left.items && left.len) pyrite_memcpy(out.items, left.items, left.len);
    if (right.items && right.len) pyrite_memcpy(out.items + left.len, right.items, right.len);
    return out;
}

static char *pyrite_bytes_to_string(PyriteBytes value) {
    char *out = pyrite_malloc(value.len + 1);
    if (!out) return "";
    if (value.items && value.len) pyrite_memcpy(out, value.items, value.len);
    out[value.len] = '\0';
    return out;
}

static PyriteList pyrite_list_int_copy(PyriteList list) {
    PyriteList out = {0};
    if (list.len == 0) return out;
    out.items = pyrite_malloc(sizeof(long) * list.len);
    if (!out.items) return out;
    out.len = list.len;
    out.cap = list.len;
    pyrite_memcpy(out.items, list.items, sizeof(long) * list.len);
    return out;
}

static long pyrite_list_int_len(PyriteList list) { return (long)list.len; }
static long pyrite_list_int_get(PyriteList list, long index) {
    if (index < 0 || (size_t)index >= list.len || !list.items) return 0;
    return list.items[index];
}
static long pyrite_list_int_peek(PyriteList list) {
    if (list.len == 0 || !list.items) return 0;
    return list.items[list.len - 1];
}
static PyriteList pyrite_list_int_push(PyriteList list, long value) {
    PyriteList out = pyrite_list_int_copy(list);
    size_t next_len = out.len + 1;
    long *items = pyrite_malloc(sizeof(long) * next_len);
    if (!items) return out;
    if (out.items && out.len) pyrite_memcpy(items, out.items, sizeof(long) * out.len);
    items[out.len] = value;
    out.items = items;
    out.len = next_len;
    out.cap = next_len;
    return out;
}
static PyriteList pyrite_list_int_set(PyriteList list, long index, long value) {
    PyriteList out = pyrite_list_int_copy(list);
    if (index >= 0 && (size_t)index < out.len) out.items[index] = value;
    return out;
}
static PyriteList pyrite_list_int_pop(PyriteList list) {
    PyriteList out = {0};
    if (list.len == 0) return out;
    out.len = list.len - 1;
    out.cap = out.len;
    if (out.len == 0) return out;
    out.items = pyrite_malloc(sizeof(long) * out.len);
    if (!out.items) return (PyriteList){0};
    pyrite_memcpy(out.items, list.items, sizeof(long) * out.len);
    return out;
}

static PyriteAnyList pyrite_list_any_copy(PyriteAnyList list) {
    PyriteAnyList out = {0};
    if (list.len == 0) return out;
    out.items = pyrite_malloc(sizeof(PyriteAny) * list.len);
    if (!out.items) return out;
    out.len = list.len;
    out.cap = list.len;
    for (size_t i = 0; i < list.len; i++) out.items[i] = pyrite_any_clone(list.items[i]);
    return out;
}
static long pyrite_list_any_len(PyriteAnyList list) { return (long)list.len; }
static PyriteAny pyrite_list_any_get(PyriteAnyList list, long index) {
    if (index < 0 || (size_t)index >= list.len || !list.items) return (PyriteAny){.kind=PYRITE_ANY_INT, .as.i=0};
    return list.items[index];
}
static PyriteAny pyrite_list_any_peek(PyriteAnyList list) {
    if (list.len == 0 || !list.items) return (PyriteAny){.kind=PYRITE_ANY_INT, .as.i=0};
    return list.items[list.len - 1];
}
static PyriteAnyList pyrite_list_any_push(PyriteAnyList list, PyriteAny value) {
    PyriteAnyList out = pyrite_list_any_copy(list);
    size_t next_len = out.len + 1;
    PyriteAny *items = pyrite_malloc(sizeof(PyriteAny) * next_len);
    if (!items) return out;
    for (size_t i = 0; i < out.len; i++) items[i] = out.items[i];
    items[out.len] = pyrite_any_clone(value);
    out.items = items;
    out.len = next_len;
    out.cap = next_len;
    return out;
}
static PyriteAnyList pyrite_list_any_set(PyriteAnyList list, long index, PyriteAny value) {
    PyriteAnyList out = pyrite_list_any_copy(list);
    if (index >= 0 && (size_t)index < out.len) {
        pyrite_release_any(&out.items[index]);
        out.items[index] = pyrite_any_clone(value);
    }
    return out;
}
static PyriteAnyList pyrite_list_any_pop(PyriteAnyList list) {
    PyriteAnyList out = {0};
    if (list.len == 0) return out;
    out.len = list.len - 1;
    out.cap = out.len;
    if (out.len == 0) return out;
    out.items = pyrite_malloc(sizeof(PyriteAny) * out.len);
    if (!out.items) return (PyriteAnyList){0};
    for (size_t i = 0; i < out.len; i++) out.items[i] = pyrite_any_clone(list.items[i]);
    return out;
}

static PyriteDict pyrite_dict_new(void) { return (PyriteDict){0}; }
static long pyrite_dict_len(PyriteDict dict) { return (long)dict.len; }
static long pyrite_dict_find(PyriteDict dict, const char *key) {
    if (!key) key = "";
    for (size_t i = 0; i < dict.len; i++) if (dict.entries[i].key && strcmp(dict.entries[i].key, key) == 0) return (long)i;
    return -1;
}
static PyriteDict pyrite_dict_copy(PyriteDict dict) {
    PyriteDict out = {0};
    if (dict.len == 0) return out;
    out.entries = pyrite_calloc(dict.len, sizeof(PyriteDictEntry));
    if (!out.entries) return out;
    out.len = dict.len;
    out.cap = dict.len;
    for (size_t i = 0; i < dict.len; i++) {
        out.entries[i].key = pyrite_promote_string(dict.entries[i].key);
        out.entries[i].value = pyrite_any_clone(dict.entries[i].value);
    }
    return out;
}
static int pyrite_dict_has(PyriteDict dict, const char *key) { return pyrite_dict_find(dict, key) >= 0; }
static PyriteAny pyrite_dict_get(PyriteDict dict, const char *key) {
    long index = pyrite_dict_find(dict, key);
    if (index < 0) return (PyriteAny){.kind=PYRITE_ANY_INT, .as.i=0};
    return dict.entries[index].value;
}
static char *pyrite_dict_get_string(PyriteDict dict, const char *key) { return pyrite_any_string(pyrite_dict_get(dict, key)); }
static long pyrite_dict_get_int(PyriteDict dict, const char *key) {
    PyriteAny value = pyrite_dict_get(dict, key);
    if (value.kind == PYRITE_ANY_INT) return value.as.i;
    if (value.kind == PYRITE_ANY_BOOL) return value.as.b;
    if (value.kind == PYRITE_ANY_FLOAT) return (long)value.as.f;
    return 0;
}
static PyriteDict pyrite_dict_set(PyriteDict dict, const char *key, PyriteAny value) {
    PyriteDict out = pyrite_dict_copy(dict);
    long index = pyrite_dict_find(out, key);
    if (index >= 0) {
        pyrite_release_any(&out.entries[index].value);
        out.entries[index].value = pyrite_any_clone(value);
        return out;
    }
    PyriteDictEntry *entries = pyrite_calloc(out.len + 1, sizeof(PyriteDictEntry));
    if (!entries) return out;
    for (size_t i = 0; i < out.len; i++) entries[i] = out.entries[i];
    entries[out.len].key = pyrite_promote_string(key);
    entries[out.len].value = pyrite_any_clone(value);
    out.entries = entries;
    out.len++;
    out.cap = out.len;
    return out;
}
static PyriteDict pyrite_dict_remove(PyriteDict dict, const char *key) {
    long drop = pyrite_dict_find(dict, key);
    if (drop < 0) return pyrite_dict_copy(dict);
    PyriteDict out = {0};
    if (dict.len <= 1) return out;
    out.entries = pyrite_calloc(dict.len - 1, sizeof(PyriteDictEntry));
    if (!out.entries) return out;
    out.len = dict.len - 1;
    out.cap = out.len;
    size_t j = 0;
    for (size_t i = 0; i < dict.len; i++) {
        if ((long)i == drop) continue;
        out.entries[j].key = pyrite_promote_string(dict.entries[i].key);
        out.entries[j].value = pyrite_any_clone(dict.entries[i].value);
        j++;
    }
    return out;
}
static void pyrite_release_dict(PyriteDict *dict) {
    (void)dict;
}

static PyriteSet pyrite_set_new(void) { return (PyriteSet){0}; }
static long pyrite_set_len(PyriteSet set) { return (long)set.len; }
static long pyrite_set_find(PyriteSet set, const char *value) {
    if (!value) value = "";
    for (size_t i = 0; i < set.len; i++) if (set.items[i] && strcmp(set.items[i], value) == 0) return (long)i;
    return -1;
}
static int pyrite_set_has(PyriteSet set, const char *value) { return pyrite_set_find(set, value) >= 0; }
static PyriteSet pyrite_set_copy(PyriteSet set) {
    PyriteSet out = {0};
    if (set.len == 0) return out;
    out.items = pyrite_calloc(set.len, sizeof(char *));
    if (!out.items) return out;
    out.len = set.len;
    out.cap = set.len;
    for (size_t i = 0; i < set.len; i++) out.items[i] = pyrite_promote_string(set.items[i]);
    return out;
}
static PyriteSet pyrite_set_add(PyriteSet set, const char *value) {
    if (pyrite_set_has(set, value)) return pyrite_set_copy(set);
    PyriteSet out = pyrite_set_copy(set);
    char **items = pyrite_calloc(out.len + 1, sizeof(char *));
    if (!items) return out;
    for (size_t i = 0; i < out.len; i++) items[i] = out.items[i];
    items[out.len] = pyrite_promote_string(value);
    out.items = items;
    out.len++;
    out.cap = out.len;
    return out;
}
static PyriteSet pyrite_set_remove(PyriteSet set, const char *value) {
    long drop = pyrite_set_find(set, value);
    if (drop < 0) return pyrite_set_copy(set);
    PyriteSet out = {0};
    if (set.len <= 1) return out;
    out.items = pyrite_calloc(set.len - 1, sizeof(char *));
    if (!out.items) return out;
    out.len = set.len - 1;
    out.cap = out.len;
    size_t j = 0;
    for (size_t i = 0; i < set.len; i++) if ((long)i != drop) out.items[j++] = pyrite_promote_string(set.items[i]);
    return out;
}
static void pyrite_release_set(PyriteSet *set) { (void)set; }

static PyriteStringBuilder pyrite_string_builder_new(void) { return (PyriteStringBuilder){0}; }
static long pyrite_string_builder_len(PyriteStringBuilder builder) { return (long)builder.len; }
static PyriteStringBuilder pyrite_string_builder_write(PyriteStringBuilder builder, const char *value) {
    if (!value) value = "";
    size_t n = pyrite_strlen(value);
    PyriteStringBuilder out = {0};
    out.len = builder.len + n;
    out.cap = out.len;
    out.items = pyrite_malloc(out.len + 1);
    if (!out.items) return out;
    if (builder.items && builder.len) pyrite_memcpy(out.items, builder.items, builder.len);
    pyrite_memcpy(out.items + builder.len, value, n);
    out.items[out.len] = '\0';
    return out;
}
static char *pyrite_string_builder_string(PyriteStringBuilder builder) {
    if (!builder.items) return "";
    return pyrite_promote_string(builder.items);
}

static PyriteBytesBuilder pyrite_bytes_builder_new(void) { return (PyriteBytesBuilder){0}; }
static long pyrite_bytes_builder_len(PyriteBytesBuilder builder) { return (long)builder.len; }
static PyriteBytesBuilder pyrite_bytes_builder_write(PyriteBytesBuilder builder, PyriteBytes value) { return pyrite_bytes_concat(builder, value); }
static PyriteBytesBuilder pyrite_bytes_builder_push(PyriteBytesBuilder builder, long value) { return pyrite_bytes_push(builder, value); }
static PyriteBytes pyrite_bytes_builder_bytes(PyriteBytesBuilder builder) { return pyrite_bytes_copy(builder); }

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
    case PYRITE_ANY_BYTES:
        return pyrite_bytes_to_string(value.as.bytes);
    case PYRITE_ANY_CLASS:
        return value.as.obj && value.as.obj->class_name ? value.as.obj->class_name : "<object>";
    default:
        return "";
    }
}

static long pyrite_any_as_int(PyriteAny value) {
    if (value.kind == PYRITE_ANY_INT) return value.as.i;
    if (value.kind == PYRITE_ANY_BOOL) return value.as.b;
    if (value.kind == PYRITE_ANY_FLOAT) return (long)value.as.f;
    return 0;
}

static double pyrite_any_as_float(PyriteAny value) {
    if (value.kind == PYRITE_ANY_FLOAT) return value.as.f;
    if (value.kind == PYRITE_ANY_INT) return (double)value.as.i;
    if (value.kind == PYRITE_ANY_BOOL) return (double)value.as.b;
    return 0.0;
}

static int pyrite_any_as_bool(PyriteAny value) {
    if (value.kind == PYRITE_ANY_BOOL) return value.as.b;
    if (value.kind == PYRITE_ANY_INT) return value.as.i != 0;
    if (value.kind == PYRITE_ANY_FLOAT) return value.as.f != 0.0;
    if (value.kind == PYRITE_ANY_STRING) return value.as.s && value.as.s[0] != '\0';
    if (value.kind == PYRITE_ANY_BYTES) return value.as.bytes.len != 0;
    return value.kind == PYRITE_ANY_CLASS && value.as.obj != 0;
}

static char *pyrite_any_as_string(PyriteAny value) {
    if (value.kind == PYRITE_ANY_STRING) return value.as.s ? value.as.s : "";
    return pyrite_any_string(value);
}

static PyriteBytes pyrite_any_as_bytes(PyriteAny value) {
    if (value.kind == PYRITE_ANY_BYTES) return pyrite_bytes_copy(value.as.bytes);
    return (PyriteBytes){0};
}

static PyriteClassObject *pyrite_any_as_class(PyriteAny value, const char *class_name) {
    if (value.kind != PYRITE_ANY_CLASS || !value.as.obj) return 0;
    if (class_name && value.as.obj->class_name && strcmp(value.as.obj->class_name, class_name) != 0) return 0;
    return value.as.obj;
}

static char *pyrite_bytes_string(PyriteBytes *bytes) {
    char *buf = pyrite_temp_alloc(512);
    char *cursor = buf;
    char *end = buf ? buf + 512 : NULL;
    if (!buf) return "";
    pyrite_append_str(&cursor, end, "b[");
    for (size_t i = 0; bytes && i < bytes->len; i++) {
        if (i) pyrite_append_str(&cursor, end, ", ");
        pyrite_append_long(&cursor, end, (long)bytes->items[i]);
    }
    pyrite_append_char(&cursor, end, ']');
    return buf;
}

static char *pyrite_dict_string(PyriteDict *dict) {
    char *buf = pyrite_temp_alloc(512);
    char *cursor = buf;
    char *end = buf ? buf + 512 : NULL;
    if (!buf) return "";
    pyrite_append_char(&cursor, end, '{');
    for (size_t i = 0; dict && i < dict->len; i++) {
        if (i) pyrite_append_str(&cursor, end, ", ");
        pyrite_append_str(&cursor, end, dict->entries[i].key ? dict->entries[i].key : "");
        pyrite_append_str(&cursor, end, ": ");
        pyrite_append_str(&cursor, end, pyrite_any_string(dict->entries[i].value));
    }
    pyrite_append_char(&cursor, end, '}');
    return buf;
}

static char *pyrite_set_string(PyriteSet *set) {
    char *buf = pyrite_temp_alloc(512);
    char *cursor = buf;
    char *end = buf ? buf + 512 : NULL;
    if (!buf) return "";
    pyrite_append_str(&cursor, end, "set[");
    for (size_t i = 0; set && i < set->len; i++) {
        if (i) pyrite_append_str(&cursor, end, ", ");
        pyrite_append_str(&cursor, end, set->items[i] ? set->items[i] : "");
    }
    pyrite_append_char(&cursor, end, ']');
    return buf;
}

static void pyrite_print_any(PyriteAny value) {
    pyrite_print_str(pyrite_any_string(value));
}

static void pyrite_release_any(PyriteAny *value) {
    (void)value;
}

static PyriteAny pyrite_any_clone(PyriteAny value) {
    if (value.kind == PYRITE_ANY_STRING) {
        return (PyriteAny){.kind=PYRITE_ANY_STRING, .as.s=pyrite_promote_string(value.as.s)};
    }
    if (value.kind == PYRITE_ANY_BYTES) {
        return (PyriteAny){.kind=PYRITE_ANY_BYTES, .as.bytes=pyrite_bytes_copy(value.as.bytes)};
    }
    return value;
}

static PyriteClassObject *pyrite_class_new(const char *class_name) {
    PyriteClassObject *obj = pyrite_calloc(1, sizeof(PyriteClassObject));
    if (!obj) return 0;
    obj->class_name = pyrite_promote_string(class_name ? class_name : "");
    return obj;
}

static PyriteClassField *pyrite_class_find_field(PyriteClassObject *obj, const char *name) {
    if (!obj || !name) return 0;
    for (size_t i = 0; i < obj->len; i++) {
        if (strcmp(obj->fields[i].name, name) == 0) return &obj->fields[i];
    }
    return 0;
}

static void pyrite_class_set(PyriteClassObject *obj, const char *name, PyriteAny value) {
    if (!obj || !name) return;
    PyriteClassField *field = pyrite_class_find_field(obj, name);
    if (field) {
        field->value = value;
        return;
    }
    if (obj->len == obj->cap) {
        size_t next = obj->cap ? obj->cap * 2 : 4;
        PyriteClassField *fields = pyrite_calloc(next, sizeof(PyriteClassField));
        if (!fields) return;
        for (size_t i = 0; i < obj->len; i++) fields[i] = obj->fields[i];
        obj->fields = fields;
        obj->cap = next;
    }
    obj->fields[obj->len].name = pyrite_promote_string(name);
    obj->fields[obj->len].value = value;
    obj->len++;
}

static PyriteAny pyrite_class_get_any(PyriteClassObject *obj, const char *name) {
    PyriteClassField *field = pyrite_class_find_field(obj, name);
    if (!field) return (PyriteAny){.kind=PYRITE_ANY_STRING, .as.s=""};
    return field->value;
}

static char *pyrite_class_get_string(PyriteClassObject *obj, const char *name) {
    PyriteAny value = pyrite_class_get_any(obj, name);
    if (value.kind == PYRITE_ANY_STRING) return value.as.s ? value.as.s : "";
    return pyrite_any_string(value);
}

static long pyrite_class_get_int(PyriteClassObject *obj, const char *name) {
    return pyrite_any_as_int(pyrite_class_get_any(obj, name));
}

static double pyrite_class_get_float(PyriteClassObject *obj, const char *name) {
    return pyrite_any_as_float(pyrite_class_get_any(obj, name));
}

static int pyrite_class_get_bool(PyriteClassObject *obj, const char *name) {
    return pyrite_any_as_bool(pyrite_class_get_any(obj, name));
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
    list.cap = (size_t)len;
    pyrite_memcpy(list.items, items, sizeof(long) * (size_t)len);
    return list;
}

static PyriteAnyList pyrite_list_any_new(PyriteAny *items, long len) {
    PyriteAnyList list = {0};
    if (len <= 0) return list;
    list.items = pyrite_malloc(sizeof(PyriteAny) * (size_t)len);
    if (!list.items) return list;
    list.len = (size_t)len;
    list.cap = (size_t)len;
    pyrite_memcpy(list.items, items, sizeof(PyriteAny) * (size_t)len);
    return list;
}

static PyriteAnyList pyrite_list_int_to_any(PyriteList list) {
    PyriteAnyList out = {0};
    if (list.len == 0) return out;
    out.items = pyrite_malloc(sizeof(PyriteAny) * list.len);
    if (!out.items) return out;
    out.len = list.len;
    out.cap = list.len;
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

static long pyrite_kernel_outw(long port, long value) {
#if defined(__x86_64__) || defined(__i386__)
    __asm__ volatile("outw %0, %1" : : "a"((uint16_t)value), "Nd"((uint16_t)port));
#else
    (void)port;
    (void)value;
#endif
    return 0;
}

static long pyrite_kernel_inw(long port) {
    uint16_t value = 0;
#if defined(__x86_64__) || defined(__i386__)
    __asm__ volatile("inw %1, %0" : "=a"(value) : "Nd"((uint16_t)port));
#else
    (void)port;
#endif
    return (long)value;
}

static long pyrite_kernel_read64(long address) {
    volatile uint64_t *ptr = (volatile uint64_t *)(uintptr_t)address;
    return (long)(*ptr);
}

static long pyrite_kernel_write64(long address, long value) {
    volatile uint64_t *ptr = (volatile uint64_t *)(uintptr_t)address;
    *ptr = (uint64_t)value;
    return 0;
}

static long pyrite_kernel_read8(long address) {
    volatile uint8_t *ptr = (volatile uint8_t *)(uintptr_t)address;
    return (long)(*ptr);
}

static long pyrite_kernel_write8(long address, long value) {
    volatile uint8_t *ptr = (volatile uint8_t *)(uintptr_t)address;
    *ptr = (uint8_t)value;
    return 0;
}

static long pyrite_kernel_read_cr3(void) {
    uint64_t value = 0;
#if defined(__x86_64__) || defined(__i386__)
    __asm__ volatile("mov %%cr3, %0" : "=r"(value));
#endif
    return (long)value;
}

static long pyrite_kernel_write_cr3(long value) {
#if defined(__x86_64__) || defined(__i386__)
    __asm__ volatile("mov %0, %%cr3" : : "r"((uint64_t)value) : "memory");
#else
    (void)value;
#endif
    return 0;
}

static long pyrite_kernel_flush_page(long address) {
#if defined(__x86_64__) || defined(__i386__)
    __asm__ volatile("invlpg (%0)" : : "r"((void *)(uintptr_t)address) : "memory");
#else
    (void)address;
#endif
    return 0;
}

static long pyrite_kernel_shr(long value, long bits) {
    return (long)((uint64_t)value >> (uint64_t)bits);
}

static long pyrite_kernel_shl(long value, long bits) {
    return (long)((uint64_t)value << (uint64_t)bits);
}

static long pyrite_kernel_ptr(const char *value) {
    return (long)(uintptr_t)value;
}

static char *pyrite_kernel_string_at(long address) {
    if (address == 0) return "";
    return (char *)(uintptr_t)address;
}

static long pyrite_kernel_string_byte(const char *value, long index) {
    if (!value || index < 0) return 0;
    size_t len = pyrite_strlen(value);
    if ((size_t)index >= len) return 0;
    return (long)(uint8_t)value[index];
}
`
