package main

const runtimeC = `
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>
#include <regex.h>
#include <time.h>
#include <errno.h>
#include <math.h>
#include <pthread.h>
#include <limits.h>
#include <unistd.h>
#include <fcntl.h>
#include <poll.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <netdb.h>
#include <netinet/tcp.h>

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
typedef struct PyriteAny PyriteAny;

typedef struct {
    PyriteAny *items;
    size_t len;
    size_t cap;
} PyriteAnyList;

typedef enum {
    PYRITE_ANY_NONE,
    PYRITE_ANY_INT,
    PYRITE_ANY_FLOAT,
    PYRITE_ANY_BOOL,
    PYRITE_ANY_STRING,
    PYRITE_ANY_BYTES,
    PYRITE_ANY_LIST,
    PYRITE_ANY_CLASS
} PyriteAnyKind;

struct PyriteAny {
    PyriteAnyKind kind;
    union {
        long i;
        double f;
        int b;
        char *s;
        PyriteBytes bytes;
        PyriteAnyList *list;
        PyriteClassObject *obj;
    } as;
};

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
static PyriteAnyList pyrite_list_any_copy(PyriteAnyList list);
static PyriteAnyList *pyrite_list_any_box(PyriteAnyList list);
static char *pyrite_list_any_string(PyriteAnyList *list);
static char *pyrite_fmt(const char *fmt, ...);
static int pyrite_argc = 0;
static char **pyrite_argv = NULL;
static int pyrite_class_object_keeps(PyriteClassObject *obj, void *ptr);

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

static __thread PyriteAllocation *pyrite_allocations = NULL;
static __thread PyriteAllocation *pyrite_allocation_nodes = NULL;
static __thread size_t pyrite_allocation_node_count = 0;
#define PYRITE_MAX_CACHED_ALLOCATION_NODES 4096
static __thread char *pyrite_temp_arena = NULL;
static __thread size_t pyrite_temp_capacity = 0;
static __thread size_t pyrite_temp_used = 0;
static char pyrite_last_error[512] = "";

static void pyrite_set_error(const char *message) {
    snprintf(pyrite_last_error, sizeof(pyrite_last_error), "%s", message ? message : "");
}

static void pyrite_set_errno_error(const char *prefix) {
    snprintf(pyrite_last_error, sizeof(pyrite_last_error), "%s: %s", prefix ? prefix : "error", strerror(errno));
}

static char *pyrite_last_error_or(const char *fallback) {
    return pyrite_last_error[0] ? pyrite_last_error : (char *)(fallback ? fallback : "runtime error");
}

static char *pyrite_last_error_value(void) {
    return pyrite_last_error_or("");
}

static void pyrite_track_allocation(void *ptr, size_t bytes) {
    if (!ptr) return;
    PyriteAllocation *node = pyrite_allocation_nodes;
    if (node) {
        pyrite_allocation_nodes = node->next;
        pyrite_allocation_node_count--;
    } else {
        node = malloc(sizeof(PyriteAllocation));
    }
    if (!node) return;
    node->ptr = ptr;
    node->bytes = bytes;
    node->next = pyrite_allocations;
    pyrite_allocations = node;
}

static void pyrite_recycle_allocation_node(PyriteAllocation *node) {
    if (!node) return;
    if (pyrite_allocation_node_count >= PYRITE_MAX_CACHED_ALLOCATION_NODES) {
        free(node);
        return;
    }
    node->ptr = NULL;
    node->bytes = 0;
    node->next = pyrite_allocation_nodes;
    pyrite_allocation_nodes = node;
    pyrite_allocation_node_count++;
}

static void pyrite_release_cached_allocation_nodes(void) {
    PyriteAllocation *node = pyrite_allocation_nodes;
    while (node) {
        PyriteAllocation *next = node->next;
        free(node);
        node = next;
    }
    pyrite_allocation_nodes = NULL;
    pyrite_allocation_node_count = 0;
}

static int pyrite_is_tracked(void *ptr) {
    for (PyriteAllocation *node = pyrite_allocations; node; node = node->next) {
        if (node->ptr == ptr) return 1;
    }
    return 0;
}

static size_t pyrite_tracked_size(void *ptr) {
    for (PyriteAllocation *node = pyrite_allocations; node; node = node->next) {
        if (node->ptr == ptr) return node->bytes;
    }
    return 0;
}

static void pyrite_release(void *ptr) {
    if (!ptr) return;
    PyriteAllocation **link = &pyrite_allocations;
    while (*link) {
        PyriteAllocation *node = *link;
        if (node->ptr == ptr) {
            *link = node->next;
            free(node->ptr);
            pyrite_recycle_allocation_node(node);
            return;
        }
        link = &node->next;
    }
}

static PyriteAllocation *pyrite_checkpoint(void) {
    return pyrite_allocations;
}

static void pyrite_release_since(PyriteAllocation *checkpoint, void *keep) {
    PyriteAllocation **link = &pyrite_allocations;
    while (*link && *link != checkpoint) {
        PyriteAllocation *node = *link;
        if (node->ptr == keep) {
            link = &node->next;
            continue;
        }
        *link = node->next;
        free(node->ptr);
        pyrite_recycle_allocation_node(node);
    }
}

static int pyrite_any_list_keeps(PyriteAnyList *list, void *ptr) {
    if (!list || !ptr) return 0;
    if (ptr == list->items) return 1;
    for (size_t i = 0; i < list->len; i++) {
        if (list->items[i].kind == PYRITE_ANY_STRING && ptr == list->items[i].as.s) return 1;
        if (list->items[i].kind == PYRITE_ANY_BYTES && ptr == list->items[i].as.bytes.items) return 1;
        if (list->items[i].kind == PYRITE_ANY_LIST && (ptr == list->items[i].as.list || pyrite_any_list_keeps(list->items[i].as.list, ptr))) return 1;
        if (list->items[i].kind == PYRITE_ANY_CLASS && pyrite_class_object_keeps(list->items[i].as.obj, ptr)) return 1;
    }
    return 0;
}

static void pyrite_release_since_any_list(PyriteAllocation *checkpoint, PyriteAnyList *keep) {
    PyriteAllocation **link = &pyrite_allocations;
    while (*link && *link != checkpoint) {
        PyriteAllocation *node = *link;
        if (pyrite_any_list_keeps(keep, node->ptr)) {
            link = &node->next;
            continue;
        }
        *link = node->next;
        free(node->ptr);
        pyrite_recycle_allocation_node(node);
    }
}

static int pyrite_class_object_keeps(PyriteClassObject *obj, void *ptr) {
    if (!obj || !ptr) return 0;
    if (ptr == obj || ptr == obj->class_name || ptr == obj->fields) return 1;
    for (size_t i = 0; i < obj->len; i++) {
        if (ptr == obj->fields[i].name) return 1;
        if (obj->fields[i].value.kind == PYRITE_ANY_STRING && ptr == obj->fields[i].value.as.s) return 1;
        if (obj->fields[i].value.kind == PYRITE_ANY_BYTES && ptr == obj->fields[i].value.as.bytes.items) return 1;
        if (obj->fields[i].value.kind == PYRITE_ANY_LIST && (ptr == obj->fields[i].value.as.list || pyrite_any_list_keeps(obj->fields[i].value.as.list, ptr))) return 1;
        if (obj->fields[i].value.kind == PYRITE_ANY_CLASS && pyrite_class_object_keeps(obj->fields[i].value.as.obj, ptr)) return 1;
    }
    return 0;
}

static void pyrite_release_since_class_object(PyriteAllocation *checkpoint, PyriteClassObject *keep) {
    PyriteAllocation **link = &pyrite_allocations;
    while (*link && *link != checkpoint) {
        PyriteAllocation *node = *link;
        if (pyrite_class_object_keeps(keep, node->ptr)) {
            link = &node->next;
            continue;
        }
        *link = node->next;
        free(node->ptr);
        pyrite_recycle_allocation_node(node);
    }
}

static void *pyrite_malloc(size_t bytes) {
    void *ptr = malloc(bytes);
    pyrite_track_allocation(ptr, bytes);
    return ptr;
}

static void *pyrite_calloc(size_t count, size_t bytes) {
    void *ptr = calloc(count, bytes);
    pyrite_track_allocation(ptr, count * bytes);
    return ptr;
}

static void *pyrite_temp_alloc(size_t bytes) {
    if (bytes == 0) bytes = 1;
    size_t aligned = (bytes + 7) & ~(size_t)7;
    if (pyrite_temp_used + aligned > pyrite_temp_capacity) {
        size_t next = pyrite_temp_capacity ? pyrite_temp_capacity : 4096;
        while (next < pyrite_temp_used + aligned) next *= 2;
        char *buf = realloc(pyrite_temp_arena, next);
        if (!buf) return NULL;
        pyrite_temp_arena = buf;
        pyrite_temp_capacity = next;
    }
    void *ptr = pyrite_temp_arena + pyrite_temp_used;
    pyrite_temp_used += aligned;
    return ptr;
}

static void pyrite_temp_reset(void) {
    pyrite_temp_used = 0;
}

static char *pyrite_promote_string(const char *s) {
    if (!s) s = "";
    size_t n = strlen(s);
    char *out = pyrite_malloc(n + 1);
    if (!out) return "";
    memcpy(out, s, n + 1);
    return out;
}

static char *pyrite_string_concat(const char *left, const char *right) {
    if (!left) left = "";
    if (!right) right = "";
    size_t left_len = strlen(left);
    size_t right_len = strlen(right);
    char *out = pyrite_malloc(left_len + right_len + 1);
    if (!out) return "";
    memcpy(out, left, left_len);
    memcpy(out + left_len, right, right_len);
    out[left_len + right_len] = '\0';
    return out;
}

static void pyrite_assign_string(char **slot, const char *s) {
    if (!slot) return;
    if (!s) s = "";
    size_t n = strlen(s) + 1;
    if (*slot && pyrite_tracked_size(*slot) >= n) {
        memcpy(*slot, s, n);
        return;
    }
    pyrite_release(*slot);
    *slot = pyrite_promote_string(s);
}

static PyriteBytes pyrite_bytes_from_data(const unsigned char *data, long len) {
    PyriteBytes out = {0};
    if (len <= 0) return out;
    out.items = pyrite_malloc((size_t)len);
    if (!out.items) return out;
    out.len = (size_t)len;
    memcpy(out.items, data, (size_t)len);
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
    if (value.items && value.len) memcpy(out.items, value.items, value.len);
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
    if (left.items && left.len) memcpy(out.items, left.items, left.len);
    if (right.items && right.len) memcpy(out.items + left.len, right.items, right.len);
    return out;
}

static char *pyrite_bytes_to_string(PyriteBytes value) {
    char *out = pyrite_malloc(value.len + 1);
    if (!out) return "";
    if (value.items && value.len) memcpy(out, value.items, value.len);
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
    memcpy(out.items, list.items, sizeof(long) * list.len);
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
    if (out.items && out.len) memcpy(items, out.items, sizeof(long) * out.len);
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
    memcpy(out.items, list.items, sizeof(long) * out.len);
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
static PyriteAnyList *pyrite_list_any_box(PyriteAnyList list) {
    PyriteAnyList *boxed = pyrite_malloc(sizeof(PyriteAnyList));
    if (!boxed) return NULL;
    *boxed = pyrite_list_any_copy(list);
    return boxed;
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
static char *pyrite_dict_get_string(PyriteDict dict, const char *key) {
    return pyrite_any_string(pyrite_dict_get(dict, key));
}
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
    if (!dict) return;
    for (size_t i = 0; i < dict->len; i++) {
        pyrite_release(dict->entries[i].key);
        pyrite_release_any(&dict->entries[i].value);
    }
    pyrite_release(dict->entries);
    dict->entries = NULL;
    dict->len = 0;
    dict->cap = 0;
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
static void pyrite_release_set(PyriteSet *set) {
    if (!set) return;
    for (size_t i = 0; i < set->len; i++) pyrite_release(set->items[i]);
    pyrite_release(set->items);
    set->items = NULL;
    set->len = 0;
    set->cap = 0;
}

static char *pyrite_dict_string(PyriteDict *dict) {
    if (!dict || dict->len == 0) return pyrite_fmt("{}");
    size_t cap = 2;
    for (size_t i = 0; i < dict->len; i++) cap += strlen(dict->entries[i].key ? dict->entries[i].key : "") + strlen(pyrite_any_string(dict->entries[i].value)) + 6;
    char *buf = pyrite_temp_alloc(cap);
    if (!buf) return "";
    size_t used = 0;
    used += snprintf(buf + used, cap - used, "{");
    for (size_t i = 0; i < dict->len; i++) {
        used += snprintf(buf + used, cap - used, "%s%s: %s", i == 0 ? "" : ", ", dict->entries[i].key ? dict->entries[i].key : "", pyrite_any_string(dict->entries[i].value));
    }
    snprintf(buf + used, cap - used, "}");
    return buf;
}

static char *pyrite_set_string(PyriteSet *set) {
    if (!set || set->len == 0) return pyrite_fmt("set[]");
    size_t cap = 5;
    for (size_t i = 0; i < set->len; i++) cap += strlen(set->items[i] ? set->items[i] : "") + 4;
    char *buf = pyrite_temp_alloc(cap);
    if (!buf) return "";
    size_t used = 0;
    used += snprintf(buf + used, cap - used, "set[");
    for (size_t i = 0; i < set->len; i++) used += snprintf(buf + used, cap - used, "%s%s", i == 0 ? "" : ", ", set->items[i] ? set->items[i] : "");
    snprintf(buf + used, cap - used, "]");
    return buf;
}

static PyriteStringBuilder pyrite_string_builder_new(void) { return (PyriteStringBuilder){0}; }
static long pyrite_string_builder_len(PyriteStringBuilder builder) { return (long)builder.len; }
static PyriteStringBuilder pyrite_string_builder_write(PyriteStringBuilder builder, const char *value) {
    if (!value) value = "";
    size_t n = strlen(value);
    PyriteStringBuilder out = {0};
    out.len = builder.len + n;
    out.cap = out.len;
    out.items = pyrite_malloc(out.len + 1);
    if (!out.items) return out;
    if (builder.items && builder.len) memcpy(out.items, builder.items, builder.len);
    memcpy(out.items + builder.len, value, n);
    out.items[out.len] = '\0';
    return out;
}
static char *pyrite_string_builder_string(PyriteStringBuilder builder) {
    if (!builder.items) return "";
    return pyrite_promote_string(builder.items);
}

static PyriteBytesBuilder pyrite_bytes_builder_new(void) { return (PyriteBytesBuilder){0}; }
static long pyrite_bytes_builder_len(PyriteBytesBuilder builder) { return (long)builder.len; }
static PyriteBytesBuilder pyrite_bytes_builder_write(PyriteBytesBuilder builder, PyriteBytes value) {
    return pyrite_bytes_concat(builder, value);
}
static PyriteBytesBuilder pyrite_bytes_builder_push(PyriteBytesBuilder builder, long value) {
    return pyrite_bytes_push(builder, value);
}
static PyriteBytes pyrite_bytes_builder_bytes(PyriteBytesBuilder builder) {
    return pyrite_bytes_copy(builder);
}

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

typedef struct {
    pthread_mutex_t inner;
} PyriteMux;

typedef struct {
    int fd;
    int is_tcp;
    char *read_buf;
    size_t read_cap;
} PyriteSocket;

typedef struct {
    int fd;
} PyriteListener;

typedef char *(*PyriteTextHandlerFn)(char *);
typedef int (*PyriteTextCloseFn)(char *);

typedef struct {
    int fd;
    char *buf;
    size_t len;
    size_t cap;
} PyriteNetClient;

typedef struct {
    int listener_fd;
    const char *delimiter;
    size_t delimiter_len;
    PyriteTextHandlerFn handler;
    PyriteTextCloseFn should_close;
    long max_messages;
    long handled;
    int stopping;
    pthread_mutex_t lock;
} PyriteNetDelimitedServer;

typedef struct {
    PyriteNetDelimitedServer *server;
    int listener_fd;
} PyriteNetDelimitedWorker;

static PyriteMux *pyrite_mux_new(void) {
    PyriteMux *mux = pyrite_malloc(sizeof(PyriteMux));
    if (!mux) return NULL;
    pthread_mutex_init(&mux->inner, NULL);
    return mux;
}

static void pyrite_mux_lock(PyriteMux *mux) {
    if (mux) pthread_mutex_lock(&mux->inner);
}

static void pyrite_mux_unlock(PyriteMux *mux) {
    if (mux) pthread_mutex_unlock(&mux->inner);
}

static long pyrite_mux_lock_value(PyriteMux *mux) {
    pyrite_mux_lock(mux);
    return 0;
}

static long pyrite_mux_unlock_value(PyriteMux *mux) {
    pyrite_mux_unlock(mux);
    return 0;
}

static void pyrite_socket_set_tcp_nodelay(int fd) {
    if (fd < 0) return;
    int yes = 1;
    setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, &yes, sizeof(yes));
}

static int pyrite_socket_set_nonblocking_fd(int fd, int enabled) {
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags < 0) return -1;
    if (enabled) flags |= O_NONBLOCK;
    else flags &= ~O_NONBLOCK;
    return fcntl(fd, F_SETFL, flags);
}

static long pyrite_socket_set_nonblocking_value(PyriteSocket *sock, long enabled) {
    if (!sock || sock->fd < 0) return -1;
    return pyrite_socket_set_nonblocking_fd(sock->fd, enabled != 0);
}

static long pyrite_listener_set_nonblocking_value(PyriteListener *listener, long enabled) {
    if (!listener || listener->fd < 0) return -1;
    return pyrite_socket_set_nonblocking_fd(listener->fd, enabled != 0);
}

static PyriteSocket *pyrite_socket_open(const char *host, long port, int is_tcp) {
    pyrite_set_error("");
    char service[32];
    snprintf(service, sizeof(service), "%ld", port);
    struct addrinfo hints;
    memset(&hints, 0, sizeof(hints));
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = is_tcp ? SOCK_STREAM : SOCK_DGRAM;

    struct addrinfo *res = NULL;
    int gai = getaddrinfo(host, service, &hints, &res);
    if (gai != 0) {
        snprintf(pyrite_last_error, sizeof(pyrite_last_error), "net.open failed: %s", gai_strerror(gai));
        return NULL;
    }

    int fd = -1;
    for (struct addrinfo *ai = res; ai; ai = ai->ai_next) {
        fd = socket(ai->ai_family, ai->ai_socktype, ai->ai_protocol);
        if (fd < 0) continue;
        if (connect(fd, ai->ai_addr, ai->ai_addrlen) == 0) break;
        pyrite_set_errno_error("net.open failed");
        close(fd);
        fd = -1;
    }
    freeaddrinfo(res);
    if (fd < 0) {
        if (!pyrite_last_error[0]) pyrite_set_error("net.open failed");
        return NULL;
    }
    if (is_tcp) pyrite_socket_set_tcp_nodelay(fd);

    PyriteSocket *sock = pyrite_malloc(sizeof(PyriteSocket));
    if (!sock) {
        close(fd);
        pyrite_set_error("net.open failed: out of memory");
        return NULL;
    }
    sock->fd = fd;
    sock->is_tcp = is_tcp;
    sock->read_buf = NULL;
    sock->read_cap = 0;
    return sock;
}

static PyriteSocket *pyrite_socket_open_tcp(const char *host, long port) {
    return pyrite_socket_open(host, port, 1);
}

static PyriteSocket *pyrite_socket_open_udp(const char *host, long port) {
    return pyrite_socket_open(host, port, 0);
}

static PyriteListener *pyrite_socket_listen_tcp_backlog(const char *host, long port, long backlog) {
    pyrite_set_error("");
    char service[32];
    snprintf(service, sizeof(service), "%ld", port);
    struct addrinfo hints;
    memset(&hints, 0, sizeof(hints));
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;
    hints.ai_flags = AI_PASSIVE;

    struct addrinfo *res = NULL;
    const char *bind_host = (host && host[0] != '\0') ? host : NULL;
    int gai = getaddrinfo(bind_host, service, &hints, &res);
    if (gai != 0) {
        snprintf(pyrite_last_error, sizeof(pyrite_last_error), "net.listen failed: %s", gai_strerror(gai));
        return NULL;
    }

    int fd = -1;
    for (struct addrinfo *ai = res; ai; ai = ai->ai_next) {
        fd = socket(ai->ai_family, ai->ai_socktype, ai->ai_protocol);
        if (fd < 0) {
            pyrite_set_errno_error("net.listen failed");
            continue;
        }
        int yes = 1;
        setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &yes, sizeof(yes));
#ifdef SO_REUSEPORT
        setsockopt(fd, SOL_SOCKET, SO_REUSEPORT, &yes, sizeof(yes));
#endif
        if (bind(fd, ai->ai_addr, ai->ai_addrlen) == 0) {
            if (backlog < 1) backlog = 16;
            if (listen(fd, (int)backlog) == 0) break;
            pyrite_set_errno_error("net.listen failed");
        } else {
            pyrite_set_errno_error("net.listen failed");
        }
        close(fd);
        fd = -1;
    }
    freeaddrinfo(res);
    if (fd < 0) {
        if (!pyrite_last_error[0]) pyrite_set_error("net.listen failed");
        return NULL;
    }

    PyriteListener *listener = pyrite_malloc(sizeof(PyriteListener));
    if (!listener) {
        close(fd);
        pyrite_set_error("net.listen failed: out of memory");
        return NULL;
    }
    listener->fd = fd;
    return listener;
}

static PyriteListener *pyrite_socket_listen_tcp(const char *host, long port) {
    return pyrite_socket_listen_tcp_backlog(host, port, 16);
}

static PyriteSocket *pyrite_socket_accept(PyriteListener *listener) {
    pyrite_set_error("");
    if (!listener || listener->fd < 0) {
        pyrite_set_error("net.accept failed: invalid listener");
        return NULL;
    }
    int fd = accept(listener->fd, NULL, NULL);
    if (fd < 0) {
        pyrite_set_errno_error("net.accept failed");
        return NULL;
    }
    pyrite_socket_set_tcp_nodelay(fd);
    PyriteSocket *sock = pyrite_malloc(sizeof(PyriteSocket));
    if (!sock) {
        close(fd);
        pyrite_set_error("net.accept failed: out of memory");
        return NULL;
    }
    sock->fd = fd;
    sock->is_tcp = 1;
    sock->read_buf = NULL;
    sock->read_cap = 0;
    return sock;
}

static long pyrite_socket_write(PyriteSocket *sock, const char *data) {
    pyrite_set_error("");
    if (!sock || sock->fd < 0 || !data) {
        pyrite_set_error("net.write failed: invalid socket or data");
        return -1;
    }
    long n = (long)send(sock->fd, data, strlen(data), 0);
    if (n < 0) pyrite_set_errno_error("net.write failed");
    return n;
}

static long pyrite_socket_read_ready(PyriteSocket *sock, long timeout_ms) {
    if (!sock || sock->fd < 0) return 0;
    struct pollfd pfd = {.fd = sock->fd, .events = POLLIN};
    int ready = poll(&pfd, 1, (int)timeout_ms);
    return ready > 0 && (pfd.revents & POLLIN);
}

static long pyrite_listener_read_ready(PyriteListener *listener, long timeout_ms) {
    if (!listener || listener->fd < 0) return 0;
    struct pollfd pfd = {.fd = listener->fd, .events = POLLIN};
    int ready = poll(&pfd, 1, (int)timeout_ms);
    return ready > 0 && (pfd.revents & POLLIN);
}

static char *pyrite_socket_read(PyriteSocket *sock, long max_bytes) {
    pyrite_set_error("");
    if (!sock || sock->fd < 0 || max_bytes <= 0) {
        pyrite_set_error("net.read failed: invalid socket or byte count");
        char *empty = pyrite_malloc(1);
        if (empty) empty[0] = '\0';
        return empty ? empty : "";
    }
    size_t needed = (size_t)max_bytes + 1;
    if (sock->read_cap < needed) {
        char *next = realloc(sock->read_buf, needed);
        if (!next) {
            pyrite_set_error("net.read failed: out of memory");
            return "";
        }
        sock->read_buf = next;
        sock->read_cap = needed;
    }
    ssize_t n = recv(sock->fd, sock->read_buf, (size_t)max_bytes, 0);
    if (n < 0) {
        pyrite_set_errno_error("net.read failed");
        sock->read_buf[0] = '\0';
        return sock->read_buf;
    }
    sock->read_buf[n] = '\0';
    return sock->read_buf;
}

static void pyrite_socket_close(const char *name, PyriteSocket *sock) {
    if (!sock || sock->fd < 0) return;
    close(sock->fd);
    sock->fd = -1;
    free(sock->read_buf);
    sock->read_buf = NULL;
    sock->read_cap = 0;
    if (PYRITE_TRACE_DEFER) {
        fprintf(stderr, "defer: closed socket %s, cleared 0 bytes\n", name);
    }
}

static long pyrite_socket_close_value(PyriteSocket *sock) {
    if (!sock || sock->fd < 0) return 0;
    close(sock->fd);
    sock->fd = -1;
    free(sock->read_buf);
    sock->read_buf = NULL;
    sock->read_cap = 0;
    return 0;
}

static void pyrite_listener_close(const char *name, PyriteListener *listener) {
    if (!listener || listener->fd < 0) return;
    close(listener->fd);
    listener->fd = -1;
    if (PYRITE_TRACE_DEFER) {
        fprintf(stderr, "defer: closed listener %s, cleared 0 bytes\n", name);
    }
}

static long pyrite_listener_close_value(PyriteListener *listener) {
    if (!listener || listener->fd < 0) return 0;
    close(listener->fd);
    listener->fd = -1;
    return 0;
}

static char *pyrite_memmem_local(const char *haystack, size_t haystack_len, const char *needle, size_t needle_len) {
    if (!haystack || !needle || needle_len == 0 || haystack_len < needle_len) return NULL;
    for (size_t i = 0; i + needle_len <= haystack_len; i++) {
        if (memcmp(haystack + i, needle, needle_len) == 0) return (char *)(haystack + i);
    }
    return NULL;
}

static int pyrite_net_client_reserve(PyriteNetClient *client, size_t needed) {
    if (client->cap >= needed) return 1;
    size_t next = client->cap ? client->cap : 4096;
    while (next < needed) next *= 2;
    char *buf = realloc(client->buf, next);
    if (!buf) return 0;
    client->buf = buf;
    client->cap = next;
    return 1;
}

static void pyrite_net_client_close(PyriteNetClient *client) {
    if (!client || client->fd < 0) return;
    close(client->fd);
    client->fd = -1;
    free(client->buf);
    client->buf = NULL;
    client->len = 0;
    client->cap = 0;
}

static int pyrite_net_send_all(int fd, const char *data) {
    if (fd < 0 || !data) return 0;
    size_t len = strlen(data);
    size_t sent = 0;
    while (sent < len) {
        ssize_t n = send(fd, data + sent, len - sent, MSG_NOSIGNAL);
        if (n > 0) {
            sent += (size_t)n;
            continue;
        }
        if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR)) {
            struct pollfd pfd = {.fd = fd, .events = POLLOUT};
            if (poll(&pfd, 1, 1000) <= 0) return 0;
            continue;
        }
        return 0;
    }
    return 1;
}

static void pyrite_net_remove_client(PyriteNetClient *clients, struct pollfd *pfds, nfds_t *nfds, nfds_t idx) {
    if (idx == 0 || idx >= *nfds) return;
    pyrite_net_client_close(&clients[idx - 1]);
    if (idx + 1 < *nfds) {
        clients[idx - 1] = clients[*nfds - 2];
        pfds[idx] = pfds[*nfds - 1];
    }
    (*nfds)--;
}

static int pyrite_net_grow_poll(PyriteNetClient **clients, struct pollfd **pfds, nfds_t *cap) {
    nfds_t next = *cap ? (*cap * 2) : 128;
    PyriteNetClient *next_clients = realloc(*clients, sizeof(PyriteNetClient) * (next - 1));
    if (!next_clients) return 0;
    struct pollfd *next_pfds = realloc(*pfds, sizeof(struct pollfd) * next);
    if (!next_pfds) {
        *clients = next_clients;
        return 0;
    }
    for (nfds_t i = *cap ? *cap - 1 : 0; i < next - 1; i++) {
        next_clients[i] = (PyriteNetClient){.fd = -1, .buf = NULL, .len = 0, .cap = 0};
    }
    *clients = next_clients;
    *pfds = next_pfds;
    *cap = next;
    return 1;
}

static int pyrite_net_delimited_should_stop(PyriteNetDelimitedServer *server) {
    pthread_mutex_lock(&server->lock);
    int stop = server->stopping || (server->max_messages > 0 && server->handled >= server->max_messages);
    pthread_mutex_unlock(&server->lock);
    return stop;
}

static long pyrite_net_delimited_mark_handled(PyriteNetDelimitedServer *server) {
    pthread_mutex_lock(&server->lock);
    if (server->max_messages > 0 && server->handled >= server->max_messages) {
        server->stopping = 1;
        pthread_mutex_unlock(&server->lock);
        return -1;
    }
    server->handled++;
    long handled = server->handled;
    if (server->max_messages > 0 && server->handled >= server->max_messages) server->stopping = 1;
    pthread_mutex_unlock(&server->lock);
    return handled;
}

static void *pyrite_net_delimited_worker(void *arg) {
    PyriteNetDelimitedWorker *worker = arg;
    PyriteNetDelimitedServer *server = worker->server;
    nfds_t cap = 0;
    nfds_t nfds = 1;
    PyriteNetClient *clients = NULL;
    struct pollfd *pfds = NULL;
    if (!pyrite_net_grow_poll(&clients, &pfds, &cap)) return NULL;
    pfds[0] = (struct pollfd){.fd = worker->listener_fd, .events = POLLIN};

    while (!pyrite_net_delimited_should_stop(server)) {
        int ready = poll(pfds, nfds, 250);
        if (ready < 0) {
            if (errno == EINTR) continue;
            break;
        }
        if (ready == 0) continue;

        if (pfds[0].revents & POLLIN) {
            int fd = accept(worker->listener_fd, NULL, NULL);
            if (fd >= 0) {
                pyrite_socket_set_tcp_nodelay(fd);
                pyrite_socket_set_nonblocking_fd(fd, 1);
                if (nfds >= cap && !pyrite_net_grow_poll(&clients, &pfds, &cap)) {
                    close(fd);
                } else {
                    clients[nfds - 1] = (PyriteNetClient){.fd = fd, .buf = NULL, .len = 0, .cap = 0};
                    pfds[nfds] = (struct pollfd){.fd = fd, .events = POLLIN};
                    nfds++;
                }
            }
        }

        for (nfds_t idx = 1; idx < nfds;) {
            PyriteNetClient *client = &clients[idx - 1];
            short events = pfds[idx].revents;
            if (events & (POLLERR | POLLHUP | POLLNVAL)) {
                pyrite_net_remove_client(clients, pfds, &nfds, idx);
                continue;
            }
            if (!(events & POLLIN)) {
                idx++;
                continue;
            }

            char chunk[4096];
            ssize_t n = recv(client->fd, chunk, sizeof(chunk), 0);
            if (n <= 0) {
                if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR)) {
                    idx++;
                    continue;
                }
                pyrite_net_remove_client(clients, pfds, &nfds, idx);
                continue;
            }
            if (!pyrite_net_client_reserve(client, client->len + (size_t)n + 1)) {
                pyrite_net_remove_client(clients, pfds, &nfds, idx);
                continue;
            }
            memcpy(client->buf + client->len, chunk, (size_t)n);
            client->len += (size_t)n;
            client->buf[client->len] = '\0';

            int close_client = 0;
            for (;;) {
                char *end = pyrite_memmem_local(client->buf, client->len, server->delimiter, server->delimiter_len);
                if (!end) break;
                size_t frame_len = (size_t)(end - client->buf) + server->delimiter_len;

                pyrite_temp_reset();
                char *frame = pyrite_temp_alloc(frame_len + 1);
                if (!frame) {
                    close_client = 1;
                    break;
                }
                memcpy(frame, client->buf, frame_len);
                frame[frame_len] = '\0';
                int request_close = server->should_close ? server->should_close(frame) : 0;
                char *response = server->handler ? server->handler(frame) : "";
                if (!pyrite_net_send_all(client->fd, response)) {
                    close_client = 1;
                    break;
                }
                long handled = pyrite_net_delimited_mark_handled(server);
                close_client = request_close;
                size_t remaining = client->len - frame_len;
                if (remaining > 0) memmove(client->buf, client->buf + frame_len, remaining);
                client->len = remaining;
                client->buf[client->len] = '\0';
                pyrite_temp_reset();
                if (handled < 0 || close_client || pyrite_net_delimited_should_stop(server)) break;
            }
            if (close_client) {
                pyrite_net_remove_client(clients, pfds, &nfds, idx);
                continue;
            }
            idx++;
        }
    }

    for (nfds_t idx = 1; idx < nfds; idx++) pyrite_net_client_close(&clients[idx - 1]);
    free(clients);
    free(pfds);
    return NULL;
}

static long pyrite_net_event_worker_count(void) {
    const char *configured = getenv("PYRITE_NET_WORKERS");
    if (configured && configured[0]) {
        long value = strtol(configured, NULL, 10);
        if (value >= 1 && value <= 128) return value;
    }
    return 3;
}

static long pyrite_net_serve_delimited(const char *host, long port, const char *delimiter, PyriteTextHandlerFn handler, PyriteTextCloseFn should_close, long max_messages) {
    if (!delimiter || delimiter[0] == '\0') {
        pyrite_set_error("net.serve_delimited failed: delimiter cannot be empty");
        return -1;
    }
    long workers = pyrite_net_event_worker_count();
    if (workers < 1) workers = 1;
    PyriteListener **listeners = calloc((size_t)workers, sizeof(PyriteListener *));
    if (!listeners) return -1;
    long listener_count = 0;
    for (long i = 0; i < workers; i++) {
        listeners[listener_count] = pyrite_socket_listen_tcp_backlog(host, port, 1024);
        if (!listeners[listener_count] || listeners[listener_count]->fd < 0) {
            if (listener_count == 0) {
                free(listeners);
                return -1;
            }
            break;
        }
        pyrite_socket_set_nonblocking_fd(listeners[listener_count]->fd, 1);
        listener_count++;
    }

    PyriteNetDelimitedServer server = {
        .listener_fd = listeners[0]->fd,
        .delimiter = delimiter,
        .delimiter_len = strlen(delimiter),
        .handler = handler,
        .should_close = should_close,
        .max_messages = max_messages,
        .handled = 0,
        .stopping = 0,
    };
    pthread_mutex_init(&server.lock, NULL);
    pthread_t *threads = calloc((size_t)listener_count, sizeof(pthread_t));
    PyriteNetDelimitedWorker *worker_args = calloc((size_t)listener_count, sizeof(PyriteNetDelimitedWorker));
    if (!threads || !worker_args) {
        for (long i = 0; i < listener_count; i++) pyrite_listener_close_value(listeners[i]);
        free(listeners);
        free(threads);
        free(worker_args);
        pthread_mutex_destroy(&server.lock);
        return -1;
    }
    long started = 0;
    for (long i = 0; i < listener_count; i++) {
        worker_args[i] = (PyriteNetDelimitedWorker){.server = &server, .listener_fd = listeners[i]->fd};
        if (pthread_create(&threads[started], NULL, pyrite_net_delimited_worker, &worker_args[i]) == 0) started++;
    }
    for (long i = 0; i < started; i++) pthread_join(threads[i], NULL);
    free(threads);
    free(worker_args);
    for (long i = 0; i < listener_count; i++) pyrite_listener_close_value(listeners[i]);
    free(listeners);
    pthread_mutex_destroy(&server.lock);
    return server.handled;
}

static char *obj_name(PyriteObject *o) {
    for (; o; o = o->parent) if (o->has_name) return o->name;
    return "";
}

static char *obj_kind(PyriteObject *o) {
    for (; o; o = o->parent) if (o->has_kind) return o->kind;
    return "";
}

static long obj_score(PyriteObject *o) {
    for (; o; o = o->parent) if (o->has_score) return o->score;
    return 0;
}

static char *pyrite_fmt(const char *fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    int n = vsnprintf(NULL, 0, fmt, ap);
    va_end(ap);
    char *buf = pyrite_temp_alloc((size_t)n + 1);
    if (!buf) return "";
    va_start(ap, fmt);
    vsnprintf(buf, (size_t)n + 1, fmt, ap);
    va_end(ap);
    return buf;
}

static void pyrite_print_str(const char *s) { puts(s ? s : ""); }
static void pyrite_print_int(long v) { printf("%ld\n", v); }
static void pyrite_print_float(double v) { printf("%g\n", v); }

static long pyrite_arg_count(void) {
    return pyrite_argc > 0 ? (long)pyrite_argc : 0;
}

static char *pyrite_arg(long index) {
    if (index < 0 || index >= pyrite_argc || !pyrite_argv) return "";
    return pyrite_argv[index] ? pyrite_argv[index] : "";
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

static char *pyrite_any_string(PyriteAny value) {
    switch (value.kind) {
    case PYRITE_ANY_INT:
        return pyrite_fmt("%ld", value.as.i);
    case PYRITE_ANY_FLOAT:
        return pyrite_fmt("%g", value.as.f);
    case PYRITE_ANY_BOOL:
        return value.as.b ? "true" : "false";
    case PYRITE_ANY_STRING:
        return value.as.s ? value.as.s : "";
    case PYRITE_ANY_BYTES:
        return pyrite_bytes_to_string(value.as.bytes);
    case PYRITE_ANY_LIST:
        return value.as.list ? pyrite_list_any_string(value.as.list) : "[]";
    case PYRITE_ANY_CLASS:
        return value.as.obj && value.as.obj->class_name ? value.as.obj->class_name : "<object>";
    case PYRITE_ANY_NONE:
        return "None";
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
    if (value.kind == PYRITE_ANY_LIST) return value.as.list && value.as.list->len != 0;
    return value.kind == PYRITE_ANY_CLASS && value.as.obj != NULL;
}

static char *pyrite_any_as_string(PyriteAny value) {
    if (value.kind == PYRITE_ANY_STRING) return value.as.s ? value.as.s : "";
    return pyrite_any_string(value);
}

static PyriteBytes pyrite_any_as_bytes(PyriteAny value) {
    if (value.kind == PYRITE_ANY_BYTES) return pyrite_bytes_copy(value.as.bytes);
    return (PyriteBytes){0};
}

static PyriteAnyList pyrite_any_as_list(PyriteAny value) {
    if (value.kind == PYRITE_ANY_LIST && value.as.list) return pyrite_list_any_copy(*value.as.list);
    return (PyriteAnyList){0};
}

static PyriteClassObject *pyrite_any_as_class(PyriteAny value, const char *class_name) {
    if (value.kind != PYRITE_ANY_CLASS || !value.as.obj) return NULL;
    if (class_name && value.as.obj->class_name && strcmp(value.as.obj->class_name, class_name) != 0) return NULL;
    return value.as.obj;
}

static char *pyrite_bytes_string(PyriteBytes *bytes) {
    if (!bytes || bytes->len == 0) return pyrite_fmt("b[]");
    size_t cap = 3 + bytes->len * 5;
    char *buf = pyrite_temp_alloc(cap);
    if (!buf) return "";
    size_t used = 0;
    used += snprintf(buf + used, cap - used, "b[");
    for (size_t i = 0; i < bytes->len; i++) {
        used += snprintf(buf + used, cap - used, "%s%u", i == 0 ? "" : ", ", (unsigned)bytes->items[i]);
    }
    snprintf(buf + used, cap - used, "]");
    return buf;
}

static void pyrite_print_any(PyriteAny value) {
    pyrite_print_str(pyrite_any_string(value));
}

static size_t pyrite_json_escaped_len(const char *s) {
    if (!s) return 0;
    size_t n = 0;
    for (; *s; s++) {
        switch (*s) {
        case '"':
        case '\\':
        case '\n':
        case '\r':
        case '\t':
            n += 2;
            break;
        default:
            n++;
            break;
        }
    }
    return n;
}

static char *pyrite_json_escape_into(char *out, const char *s) {
    if (!s) return out;
    for (; *s; s++) {
        switch (*s) {
        case '"':
            *out++ = '\\'; *out++ = '"';
            break;
        case '\\':
            *out++ = '\\'; *out++ = '\\';
            break;
        case '\n':
            *out++ = '\\'; *out++ = 'n';
            break;
        case '\r':
            *out++ = '\\'; *out++ = 'r';
            break;
        case '\t':
            *out++ = '\\'; *out++ = 't';
            break;
        default:
            *out++ = *s;
            break;
        }
    }
    return out;
}

static char *pyrite_json_quote_string(const char *s) {
    size_t cap = pyrite_json_escaped_len(s) + 3;
    char *out = pyrite_temp_alloc(cap);
    if (!out) return "";
    char *p = out;
    *p++ = '"';
    p = pyrite_json_escape_into(p, s ? s : "");
    *p++ = '"';
    *p = '\0';
    return out;
}

static char *pyrite_json_stringify_any(PyriteAny value) {
    switch (value.kind) {
    case PYRITE_ANY_STRING:
        return pyrite_json_quote_string(value.as.s);
    case PYRITE_ANY_INT:
    case PYRITE_ANY_FLOAT:
    case PYRITE_ANY_BOOL:
        return pyrite_any_string(value);
    default:
        return "null";
    }
}

static char *pyrite_json_stringify_list(PyriteAnyList items) {
    size_t cap = 3;
    for (size_t i = 0; i < items.len; i++) {
        if (items.items[i].kind == PYRITE_ANY_STRING) {
            cap += pyrite_json_escaped_len(items.items[i].as.s) + 4;
        } else {
            cap += strlen(pyrite_any_string(items.items[i])) + 2;
        }
    }
    char *out = pyrite_temp_alloc(cap);
    if (!out) return "";
    size_t used = 0;
    used += snprintf(out + used, cap - used, "[");
    for (size_t i = 0; i < items.len; i++) {
        if (i > 0) used += snprintf(out + used, cap - used, ",");
        if (items.items[i].kind == PYRITE_ANY_STRING) {
            char *quoted = pyrite_json_quote_string(items.items[i].as.s);
            used += snprintf(out + used, cap - used, "%s", quoted);
        } else {
            used += snprintf(out + used, cap - used, "%s", pyrite_any_string(items.items[i]));
        }
    }
    snprintf(out + used, cap - used, "]");
    return out;
}

static const char *pyrite_json_skip_ws(const char *p) {
    while (p && pyrite_ascii_space((unsigned char)*p)) p++;
    return p;
}

static char *pyrite_json_parse_string_temp(const char **cursor) {
    const char *p = pyrite_json_skip_ws(*cursor);
    if (!p || *p != '"') return NULL;
    p++;
    size_t cap = strlen(p) + 1;
    char *out = pyrite_temp_alloc(cap);
    if (!out) return NULL;
    size_t used = 0;
    while (*p && *p != '"') {
        if (*p == '\\') {
            p++;
            switch (*p) {
            case 'n': out[used++] = '\n'; break;
            case 'r': out[used++] = '\r'; break;
            case 't': out[used++] = '\t'; break;
            case '"': out[used++] = '"'; break;
            case '\\': out[used++] = '\\'; break;
            default: if (*p) out[used++] = *p; break;
            }
            if (*p) p++;
            continue;
        }
        out[used++] = *p++;
    }
    if (*p != '"') return NULL;
    out[used] = '\0';
    *cursor = p + 1;
    return out;
}

static PyriteAny pyrite_json_parse_any_at(const char **cursor) {
    const char *p = pyrite_json_skip_ws(*cursor);
    pyrite_set_error("");
    if (!p) return (PyriteAny){.kind=PYRITE_ANY_INT, .as.i=0};
    if (*p == '"') {
        char *s = pyrite_json_parse_string_temp(&p);
        if (!s) {
            pyrite_set_error("json.parse failed: invalid string");
            return (PyriteAny){.kind=PYRITE_ANY_STRING, .as.s=pyrite_promote_string("")};
        }
        *cursor = p;
        return (PyriteAny){.kind=PYRITE_ANY_STRING, .as.s=pyrite_promote_string(s)};
    }
    if (strncmp(p, "true", 4) == 0) {
        *cursor = p + 4;
        return (PyriteAny){.kind=PYRITE_ANY_BOOL, .as.b=1};
    }
    if (strncmp(p, "false", 5) == 0) {
        *cursor = p + 5;
        return (PyriteAny){.kind=PYRITE_ANY_BOOL, .as.b=0};
    }
    char *end = NULL;
    double f = strtod(p, &end);
    if (end && end != p) {
        *cursor = end;
        if (strchr(p, '.') && strchr(p, '.') < end) {
            return (PyriteAny){.kind=PYRITE_ANY_FLOAT, .as.f=f};
        }
        return (PyriteAny){.kind=PYRITE_ANY_INT, .as.i=(long)f};
    }
    pyrite_set_error("json.parse failed: unsupported value");
    *cursor = p;
    return (PyriteAny){.kind=PYRITE_ANY_STRING, .as.s=pyrite_promote_string("")};
}

static PyriteAny pyrite_json_parse_any(const char *text) {
    const char *p = text ? text : "";
    return pyrite_json_parse_any_at(&p);
}

static PyriteAnyList pyrite_json_parse_array(const char *text) {
    pyrite_set_error("");
    const char *p = pyrite_json_skip_ws(text ? text : "");
    if (*p != '[') {
        pyrite_set_error("json.parse_array failed: expected [");
        return (PyriteAnyList){0};
    }
    p++;
    size_t cap = 1;
    for (const char *q = p; *q; q++) {
        if (*q == ',') cap++;
        if (*q == ']') break;
    }
    PyriteAny *items = pyrite_calloc(cap, sizeof(PyriteAny));
    if (!items) return (PyriteAnyList){0};
    size_t len = 0;
    p = pyrite_json_skip_ws(p);
    if (*p == ']') return (PyriteAnyList){items, 0, cap};
    while (*p && *p != ']') {
        items[len++] = pyrite_json_parse_any_at(&p);
        p = pyrite_json_skip_ws(p);
        if (*p == ',') {
            p++;
            continue;
        }
        if (*p != ']') {
            pyrite_set_error("json.parse_array failed: expected comma or ]");
            break;
        }
    }
    return (PyriteAnyList){items, len, cap};
}

static const char *pyrite_json_find_key_value(const char *text, const char *key) {
    const char *p = pyrite_json_skip_ws(text ? text : "");
    if (*p != '{') return NULL;
    p++;
    while (*p) {
        p = pyrite_json_skip_ws(p);
        if (*p == '}') return NULL;
        const char *key_start = p;
        char *found = pyrite_json_parse_string_temp(&p);
        if (!found) return NULL;
        p = pyrite_json_skip_ws(p);
        if (*p != ':') return NULL;
        p++;
        if (strcmp(found, key ? key : "") == 0) return pyrite_json_skip_ws(p);
        p = key_start;
        int in_string = 0;
        while (*p) {
            if (*p == '"' && (p == key_start || p[-1] != '\\')) in_string = !in_string;
            if (!in_string && (*p == ',' || *p == '}')) break;
            p++;
        }
        if (*p == ',') p++;
    }
    return NULL;
}

static char *pyrite_json_get_string(const char *text, const char *key) {
    pyrite_set_error("");
    const char *p = pyrite_json_find_key_value(text, key);
    if (!p) {
        pyrite_set_error("json.get_string failed: key not found");
        return "";
    }
    char *s = pyrite_json_parse_string_temp(&p);
    if (!s) {
        pyrite_set_error("json.get_string failed: value is not a string");
        return "";
    }
    return s;
}

static long pyrite_json_get_int(const char *text, const char *key) {
    const char *p = pyrite_json_find_key_value(text, key);
    if (!p) {
        pyrite_set_error("json.get_int failed: key not found");
        return 0;
    }
    return strtol(p, NULL, 10);
}

static double pyrite_json_get_float(const char *text, const char *key) {
    const char *p = pyrite_json_find_key_value(text, key);
    if (!p) {
        pyrite_set_error("json.get_float failed: key not found");
        return 0.0;
    }
    return strtod(p, NULL);
}

static int pyrite_json_get_bool(const char *text, const char *key) {
    const char *p = pyrite_json_find_key_value(text, key);
    if (!p) {
        pyrite_set_error("json.get_bool failed: key not found");
        return 0;
    }
    return strncmp(p, "true", 4) == 0;
}

static size_t pyrite_xml_escaped_len(const char *s) {
    if (!s) return 0;
    size_t n = 0;
    for (; *s; s++) {
        switch (*s) {
        case '&': n += 5; break;
        case '<': n += 4; break;
        case '>': n += 4; break;
        case '"': n += 6; break;
        case '\'': n += 6; break;
        default: n++; break;
        }
    }
    return n;
}

static char *pyrite_xml_escape(const char *s) {
    size_t cap = pyrite_xml_escaped_len(s) + 1;
    char *out = pyrite_temp_alloc(cap);
    if (!out) return "";
    size_t used = 0;
    for (const char *p = s ? s : ""; *p; p++) {
        const char *rep = NULL;
        switch (*p) {
        case '&': rep = "&amp;"; break;
        case '<': rep = "&lt;"; break;
        case '>': rep = "&gt;"; break;
        case '"': rep = "&quot;"; break;
        case '\'': rep = "&apos;"; break;
        default: out[used++] = *p; break;
        }
        if (rep) {
            size_t n = strlen(rep);
            memcpy(out + used, rep, n);
            used += n;
        }
    }
    out[used] = '\0';
    return out;
}

static char *pyrite_xml_unescape(const char *s) {
    size_t cap = strlen(s ? s : "") + 1;
    char *out = pyrite_temp_alloc(cap);
    if (!out) return "";
    size_t used = 0;
    for (const char *p = s ? s : ""; *p;) {
        if (strncmp(p, "&amp;", 5) == 0) { out[used++] = '&'; p += 5; }
        else if (strncmp(p, "&lt;", 4) == 0) { out[used++] = '<'; p += 4; }
        else if (strncmp(p, "&gt;", 4) == 0) { out[used++] = '>'; p += 4; }
        else if (strncmp(p, "&quot;", 6) == 0) { out[used++] = '"'; p += 6; }
        else if (strncmp(p, "&apos;", 6) == 0) { out[used++] = '\''; p += 6; }
        else { out[used++] = *p++; }
    }
    out[used] = '\0';
    return out;
}

static char *pyrite_xml_tag(const char *name, const char *text) {
    char *escaped = pyrite_xml_escape(text);
    size_t cap = strlen(name ? name : "") * 2 + strlen(escaped) + 6;
    char *out = pyrite_temp_alloc(cap);
    if (!out) return "";
    snprintf(out, cap, "<%s>%s</%s>", name ? name : "", escaped, name ? name : "");
    return out;
}

static char *pyrite_xml_text(const char *xml, const char *name) {
    const char *tag = name ? name : "";
    size_t open_len = strlen(tag) + 3;
    size_t close_len = strlen(tag) + 4;
    char *open = pyrite_temp_alloc(open_len);
    char *close = pyrite_temp_alloc(close_len);
    if (!open || !close) return "";
    snprintf(open, open_len, "<%s>", tag);
    snprintf(close, close_len, "</%s>", tag);
    const char *start = strstr(xml ? xml : "", open);
    if (!start) return "";
    start += strlen(open);
    const char *end = strstr(start, close);
    if (!end) return "";
    size_t n = (size_t)(end - start);
    char *raw = pyrite_temp_alloc(n + 1);
    if (!raw) return "";
    memcpy(raw, start, n);
    raw[n] = '\0';
    return pyrite_xml_unescape(raw);
}

static void pyrite_release_any(PyriteAny *value) {
    if (!value) return;
    if (value->kind == PYRITE_ANY_STRING) {
        pyrite_release(value->as.s);
        value->as.s = NULL;
    } else if (value->kind == PYRITE_ANY_BYTES) {
        pyrite_release(value->as.bytes.items);
        value->as.bytes.items = NULL;
        value->as.bytes.len = 0;
    } else if (value->kind == PYRITE_ANY_LIST) {
        if (value->as.list) {
            for (size_t i = 0; i < value->as.list->len; i++) pyrite_release_any(&value->as.list->items[i]);
            pyrite_release(value->as.list->items);
            pyrite_release(value->as.list);
            value->as.list = NULL;
        }
    }
}

static PyriteAny pyrite_any_clone(PyriteAny value) {
    if (value.kind == PYRITE_ANY_STRING) {
        return (PyriteAny){.kind=PYRITE_ANY_STRING, .as.s=pyrite_promote_string(value.as.s)};
    }
    if (value.kind == PYRITE_ANY_BYTES) {
        return (PyriteAny){.kind=PYRITE_ANY_BYTES, .as.bytes=pyrite_bytes_copy(value.as.bytes)};
    }
    if (value.kind == PYRITE_ANY_LIST) {
        return (PyriteAny){.kind=PYRITE_ANY_LIST, .as.list=value.as.list ? pyrite_list_any_box(*value.as.list) : NULL};
    }
    return value;
}

static PyriteClassObject *pyrite_class_new(const char *class_name) {
    PyriteClassObject *obj = pyrite_calloc(1, sizeof(PyriteClassObject));
    if (!obj) return NULL;
    obj->class_name = pyrite_promote_string(class_name ? class_name : "");
    return obj;
}

static PyriteClassField *pyrite_class_find_field(PyriteClassObject *obj, const char *name) {
    if (!obj || !name) return NULL;
    for (size_t i = 0; i < obj->len; i++) {
        if (strcmp(obj->fields[i].name, name) == 0) return &obj->fields[i];
    }
    return NULL;
}

static void pyrite_class_set(PyriteClassObject *obj, const char *name, PyriteAny value) {
    if (!obj || !name) return;
    PyriteClassField *field = pyrite_class_find_field(obj, name);
    if (field) {
        pyrite_release_any(&field->value);
        field->value = value;
        return;
    }
    if (obj->len == obj->cap) {
        size_t next = obj->cap ? obj->cap * 2 : 4;
        PyriteClassField *fields = pyrite_calloc(next, sizeof(PyriteClassField));
        if (!fields) return;
        for (size_t i = 0; i < obj->len; i++) fields[i] = obj->fields[i];
        pyrite_release(obj->fields);
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
    PyriteAny value = pyrite_class_get_any(obj, name);
    if (value.kind == PYRITE_ANY_INT) return value.as.i;
    if (value.kind == PYRITE_ANY_BOOL) return value.as.b;
    if (value.kind == PYRITE_ANY_FLOAT) return (long)value.as.f;
    return 0;
}

static double pyrite_class_get_float(PyriteClassObject *obj, const char *name) {
    PyriteAny value = pyrite_class_get_any(obj, name);
    if (value.kind == PYRITE_ANY_FLOAT) return value.as.f;
    if (value.kind == PYRITE_ANY_INT) return (double)value.as.i;
    if (value.kind == PYRITE_ANY_BOOL) return (double)value.as.b;
    return 0.0;
}

static int pyrite_class_get_bool(PyriteClassObject *obj, const char *name) {
    PyriteAny value = pyrite_class_get_any(obj, name);
    if (value.kind == PYRITE_ANY_BOOL) return value.as.b;
    if (value.kind == PYRITE_ANY_INT) return value.as.i != 0;
    if (value.kind == PYRITE_ANY_FLOAT) return value.as.f != 0.0;
    if (value.kind == PYRITE_ANY_STRING) return value.as.s && value.as.s[0] != '\0';
    return 0;
}

static void pyrite_release_any_list(PyriteAnyList *list) {
    if (!list) return;
    for (size_t i = 0; i < list->len; i++) {
        pyrite_release_any(&list->items[i]);
    }
    pyrite_release(list->items);
    list->items = NULL;
    list->len = 0;
}

static PyriteList pyrite_list_int_new(long *values, size_t len) {
    if (len == 0) return (PyriteList){NULL, 0, 0};
    long *items = pyrite_malloc(sizeof(long) * len);
    if (!items) return (PyriteList){NULL, 0, 0};
    for (size_t i = 0; i < len; i++) items[i] = values[i];
    return (PyriteList){items, len, len};
}

static PyriteAnyList pyrite_list_any_new(PyriteAny *values, size_t len) {
    if (len == 0) return (PyriteAnyList){NULL, 0, 0};
    PyriteAny *items = pyrite_malloc(sizeof(PyriteAny) * len);
    if (!items) return (PyriteAnyList){NULL, 0, 0};
    for (size_t i = 0; i < len; i++) items[i] = values[i];
    return (PyriteAnyList){items, len, len};
}

static long pyrite_int_abs(long v) {
    return v < 0 ? -v : v;
}

static long pyrite_int_min(long left, long right) {
    return left < right ? left : right;
}

static long pyrite_int_max(long left, long right) {
    return left > right ? left : right;
}

static long pyrite_int_clamp(long value, long min, long max) {
    if (max < min) {
        long tmp = min;
        min = max;
        max = tmp;
    }
    if (value < min) return min;
    if (value > max) return max;
    return value;
}

static double pyrite_int_to_float(long value) {
    return (double)value;
}

static char *pyrite_int_to_string(long value) {
    return pyrite_fmt("%ld", value);
}

static int pyrite_int_is_even(long value) {
    return value % 2 == 0;
}

static int pyrite_int_is_odd(long value) {
    return value % 2 != 0;
}

static double pyrite_float_abs(double value) {
    return fabs(value);
}

static double pyrite_float_min(double left, double right) {
    return left < right ? left : right;
}

static double pyrite_float_max(double left, double right) {
    return left > right ? left : right;
}

static double pyrite_float_clamp(double value, double min, double max) {
    if (max < min) {
        double tmp = min;
        min = max;
        max = tmp;
    }
    if (value < min) return min;
    if (value > max) return max;
    return value;
}

static long pyrite_float_round(double value) {
    return (long)llround(value);
}

static long pyrite_float_floor(double value) {
    return (long)floor(value);
}

static long pyrite_float_ceil(double value) {
    return (long)ceil(value);
}

static long pyrite_float_trunc(double value) {
    return (long)trunc(value);
}

static long pyrite_float_to_int(double value) {
    return (long)value;
}

static char *pyrite_float_to_string(double value) {
    return pyrite_fmt("%g", value);
}

static char *pyrite_string_copy_range(const char *s, size_t start, size_t end) {
    if (!s || end < start) {
        char *empty = pyrite_temp_alloc(1);
        if (empty) empty[0] = '\0';
        return empty ? empty : "";
    }
    size_t n = end - start;
    char *out = pyrite_temp_alloc(n + 1);
    if (!out) return "";
    memcpy(out, s + start, n);
    out[n] = '\0';
    return out;
}

static char *pyrite_string_slice(const char *s, long start, long end) {
    if (!s) s = "";
    size_t len = strlen(s);
    if (start < 0) start = 0;
    if (end < start) end = start;
    if ((size_t)start > len) start = (long)len;
    if ((size_t)end > len) end = (long)len;
    return pyrite_string_copy_range(s, (size_t)start, (size_t)end);
}

static char *pyrite_string_at(const char *s, long index) {
    if (!s || index < 0 || (size_t)index >= strlen(s)) return pyrite_promote_string("");
    char *out = pyrite_malloc(2);
    if (!out) return "";
    out[0] = s[index];
    out[1] = '\0';
    return out;
}

static long pyrite_string_byte_at(const char *s, long index) {
    if (!s || index < 0 || (size_t)index >= strlen(s)) return 0;
    return (long)(unsigned char)s[index];
}

static char *pyrite_string_lstrip(const char *s) {
    if (!s) s = "";
    size_t start = 0;
    size_t len = strlen(s);
    while (start < len && pyrite_ascii_space((unsigned char)s[start])) start++;
    return pyrite_string_copy_range(s, start, len);
}

static char *pyrite_string_rstrip(const char *s) {
    if (!s) s = "";
    size_t end = strlen(s);
    while (end > 0 && pyrite_ascii_space((unsigned char)s[end - 1])) end--;
    return pyrite_string_copy_range(s, 0, end);
}

static char *pyrite_string_strip(const char *s) {
    if (!s) s = "";
    size_t start = 0;
    size_t end = strlen(s);
    while (start < end && pyrite_ascii_space((unsigned char)s[start])) start++;
    while (end > start && pyrite_ascii_space((unsigned char)s[end - 1])) end--;
    return pyrite_string_copy_range(s, start, end);
}

static char *pyrite_string_upper(const char *s) {
    if (!s) s = "";
    size_t len = strlen(s);
    char *out = pyrite_temp_alloc(len + 1);
    if (!out) return "";
    for (size_t i = 0; i < len; i++) out[i] = pyrite_ascii_upper((unsigned char)s[i]);
    out[len] = '\0';
    return out;
}

static char *pyrite_string_lower(const char *s) {
    if (!s) s = "";
    size_t len = strlen(s);
    char *out = pyrite_temp_alloc(len + 1);
    if (!out) return "";
    for (size_t i = 0; i < len; i++) out[i] = pyrite_ascii_lower((unsigned char)s[i]);
    out[len] = '\0';
    return out;
}

static long pyrite_string_len(const char *s) {
    return s ? (long)strlen(s) : 0;
}

static long pyrite_string_find(const char *s, const char *needle) {
    if (!s || !needle) return -1;
    char *hit = strstr(s, needle);
    return hit ? (long)(hit - s) : -1;
}

static int pyrite_string_contains(const char *s, const char *needle) {
    return pyrite_string_find(s, needle) >= 0;
}

static int pyrite_string_startswith(const char *s, const char *prefix) {
    if (!s || !prefix) return 0;
    size_t n = strlen(prefix);
    return strncmp(s, prefix, n) == 0;
}

static int pyrite_string_endswith(const char *s, const char *suffix) {
    if (!s || !suffix) return 0;
    size_t len = strlen(s);
    size_t n = strlen(suffix);
    return n <= len && strcmp(s + len - n, suffix) == 0;
}

static char *pyrite_string_replace(const char *s, const char *old, const char *replacement) {
    if (!s) s = "";
    if (!old) old = "";
    if (!replacement) replacement = "";
    size_t old_len = strlen(old);
    if (old_len == 0) return pyrite_string_copy_range(s, 0, strlen(s));
    size_t repl_len = strlen(replacement);
    size_t count = 0;
    const char *p = s;
    while ((p = strstr(p, old))) {
        count++;
        p += old_len;
    }
    size_t len = strlen(s);
    size_t out_len = len;
    if (repl_len >= old_len) {
        out_len += count * (repl_len - old_len);
    } else {
        out_len -= count * (old_len - repl_len);
    }
    char *out = pyrite_temp_alloc(out_len + 1);
    if (!out) return "";
    char *dst = out;
    p = s;
    const char *hit;
    while ((hit = strstr(p, old))) {
        size_t chunk = (size_t)(hit - p);
        memcpy(dst, p, chunk);
        dst += chunk;
        memcpy(dst, replacement, repl_len);
        dst += repl_len;
        p = hit + old_len;
    }
    strcpy(dst, p);
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

static char *pyrite_list_int_string(PyriteList *list) {
    if (!list || list->len == 0) return pyrite_fmt("[]");
    size_t cap = 2;
    for (size_t i = 0; i < list->len; i++) cap += 24;
    char *buf = pyrite_temp_alloc(cap);
    if (!buf) return "";
    size_t used = 0;
    used += snprintf(buf + used, cap - used, "[");
    for (size_t i = 0; i < list->len; i++) {
        used += snprintf(buf + used, cap - used, "%s%ld", i == 0 ? "" : ", ", list->items[i]);
    }
    snprintf(buf + used, cap - used, "]");
    return buf;
}

static char *pyrite_list_any_string(PyriteAnyList *list) {
    if (!list || list->len == 0) return pyrite_fmt("[]");
    size_t cap = 2;
    for (size_t i = 0; i < list->len; i++) cap += strlen(pyrite_any_string(list->items[i])) + 4;
    char *buf = pyrite_temp_alloc(cap);
    if (!buf) return "";
    size_t used = 0;
    used += snprintf(buf + used, cap - used, "[");
    for (size_t i = 0; i < list->len; i++) {
        char *item = pyrite_any_string(list->items[i]);
        if (list->items[i].kind == PYRITE_ANY_STRING) {
            used += snprintf(buf + used, cap - used, "%s\"%s\"", i == 0 ? "" : ", ", item);
        } else {
            used += snprintf(buf + used, cap - used, "%s%s", i == 0 ? "" : ", ", item);
        }
    }
    snprintf(buf + used, cap - used, "]");
    return buf;
}

static PyriteAnyList pyrite_list_int_to_any(PyriteList list) {
    PyriteAny *items = pyrite_malloc(sizeof(PyriteAny) * list.len);
    if (!items) return (PyriteAnyList){0};
    for (size_t i = 0; i < list.len; i++) {
        items[i] = (PyriteAny){.kind = PYRITE_ANY_INT, .as.i = list.items[i]};
    }
    return (PyriteAnyList){items, list.len, list.len};
}

typedef enum {
    PYRITE_ROUTINE_PRINT_STR,
    PYRITE_ROUTINE_PRINT_INT,
    PYRITE_ROUTINE_PRINT_FLOAT
} PyriteRoutineKind;

static size_t pyrite_defer_memory_cleanup(void);

typedef struct {
    PyriteRoutineKind kind;
    PyriteMux *mux;
    const char *s;
    long i;
    double f;
} PyriteRoutineTask;

typedef struct PyriteQueuedTask {
    void *(*run)(void *);
    void *task;
    struct PyriteQueuedTask *next;
} PyriteQueuedTask;

static pthread_mutex_t pyrite_routine_state_lock = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t pyrite_routine_state_done = PTHREAD_COND_INITIALIZER;
static pthread_cond_t pyrite_routine_state_work = PTHREAD_COND_INITIALIZER;
static size_t pyrite_routine_active_count = 0;
static PyriteQueuedTask *pyrite_routine_queue_head = NULL;
static PyriteQueuedTask *pyrite_routine_queue_tail = NULL;
static pthread_t *pyrite_routine_workers = NULL;
static size_t pyrite_routine_worker_count = 0;
static size_t pyrite_routine_configured_workers = 0;
static int pyrite_routine_pool_started = 0;
static int pyrite_routine_pool_stopping = 0;
static void pyrite_start_routine(PyriteRoutineTask *task);
static void pyrite_start_task(void *(*run)(void *), void *task);

static size_t pyrite_default_worker_count(void) {
    const char *configured = getenv("PYRITE_WORKERS");
    if (configured && configured[0]) {
        long value = strtol(configured, NULL, 10);
        if (value >= 1 && value <= 1024) return (size_t)value;
    }
    long cpus = sysconf(_SC_NPROCESSORS_ONLN);
    if (cpus < 1) cpus = 4;
    size_t workers = (size_t)cpus * 4;
    if (workers < 4) workers = 4;
    if (workers > 64) workers = 64;
    return workers;
}

static void *pyrite_routine_run(void *arg) {
    PyriteRoutineTask *task = arg;
    pyrite_mux_lock(task->mux);
    switch (task->kind) {
    case PYRITE_ROUTINE_PRINT_STR:
        pyrite_print_str(task->s);
        break;
    case PYRITE_ROUTINE_PRINT_INT:
        pyrite_print_int(task->i);
        break;
    case PYRITE_ROUTINE_PRINT_FLOAT:
        pyrite_print_float(task->f);
        break;
    }
    pyrite_mux_unlock(task->mux);
    pyrite_defer_memory_cleanup();
    return NULL;
}

static void *pyrite_routine_worker_loop(void *arg) {
    (void)arg;
    for (;;) {
        pthread_mutex_lock(&pyrite_routine_state_lock);
        while (!pyrite_routine_queue_head && !pyrite_routine_pool_stopping) {
            pthread_cond_wait(&pyrite_routine_state_work, &pyrite_routine_state_lock);
        }
        if (!pyrite_routine_queue_head && pyrite_routine_pool_stopping) {
            pthread_mutex_unlock(&pyrite_routine_state_lock);
            return NULL;
        }
        PyriteQueuedTask *task = pyrite_routine_queue_head;
        pyrite_routine_queue_head = task->next;
        if (!pyrite_routine_queue_head) pyrite_routine_queue_tail = NULL;
        pthread_mutex_unlock(&pyrite_routine_state_lock);

        task->run(task->task);
        free(task);

        pthread_mutex_lock(&pyrite_routine_state_lock);
        if (pyrite_routine_active_count > 0) pyrite_routine_active_count--;
        if (pyrite_routine_active_count == 0) pthread_cond_broadcast(&pyrite_routine_state_done);
        pthread_mutex_unlock(&pyrite_routine_state_lock);
    }
}

static int pyrite_routine_start_pool_locked(void) {
    if (pyrite_routine_pool_started) return 1;
    size_t workers = pyrite_routine_configured_workers ? pyrite_routine_configured_workers : pyrite_default_worker_count();
    if (workers < 1) workers = 1;
    pyrite_routine_workers = calloc(workers, sizeof(pthread_t));
    if (!pyrite_routine_workers) return 0;

    pthread_attr_t attr;
    pthread_attr_init(&attr);
    size_t stack_size = 256 * 1024;
    if (stack_size < (size_t)PTHREAD_STACK_MIN) stack_size = (size_t)PTHREAD_STACK_MIN;
    pthread_attr_setstacksize(&attr, stack_size);

    pyrite_routine_pool_stopping = 0;
    pyrite_routine_worker_count = 0;
    for (size_t i = 0; i < workers; i++) {
        if (pthread_create(&pyrite_routine_workers[pyrite_routine_worker_count], &attr, pyrite_routine_worker_loop, NULL) == 0) {
            pyrite_routine_worker_count++;
        }
    }
    pthread_attr_destroy(&attr);
    if (pyrite_routine_worker_count == 0) {
        free(pyrite_routine_workers);
        pyrite_routine_workers = NULL;
        return 0;
    }
    pyrite_routine_pool_started = 1;
    return 1;
}

static void pyrite_routine_print_str(const char *s, PyriteMux *mux) {
    PyriteRoutineTask *task = pyrite_malloc(sizeof(PyriteRoutineTask));
    if (!task) return;
    *task = (PyriteRoutineTask){.kind = PYRITE_ROUTINE_PRINT_STR, .mux = mux, .s = s};
    pyrite_start_routine(task);
}

static void pyrite_routine_print_int(long v, PyriteMux *mux) {
    PyriteRoutineTask *task = pyrite_malloc(sizeof(PyriteRoutineTask));
    if (!task) return;
    *task = (PyriteRoutineTask){.kind = PYRITE_ROUTINE_PRINT_INT, .mux = mux, .i = v};
    pyrite_start_routine(task);
}

static void pyrite_routine_print_float(double v, PyriteMux *mux) {
    PyriteRoutineTask *task = pyrite_malloc(sizeof(PyriteRoutineTask));
    if (!task) return;
    *task = (PyriteRoutineTask){.kind = PYRITE_ROUTINE_PRINT_FLOAT, .mux = mux, .f = v};
    pyrite_start_routine(task);
}

static void pyrite_start_task(void *(*run)(void *), void *task) {
    if (!run || !task) return;
    PyriteQueuedTask *queued = malloc(sizeof(PyriteQueuedTask));
    if (!queued) {
        run(task);
        return;
    }
    *queued = (PyriteQueuedTask){.run = run, .task = task, .next = NULL};
    pthread_mutex_lock(&pyrite_routine_state_lock);
    if (!pyrite_routine_start_pool_locked()) {
        pthread_mutex_unlock(&pyrite_routine_state_lock);
        free(queued);
        run(task);
        return;
    }
    pyrite_routine_active_count++;
    if (pyrite_routine_queue_tail) {
        pyrite_routine_queue_tail->next = queued;
    } else {
        pyrite_routine_queue_head = queued;
    }
    pyrite_routine_queue_tail = queued;
    pthread_cond_signal(&pyrite_routine_state_work);
    pthread_mutex_unlock(&pyrite_routine_state_lock);
}

static void pyrite_start_routine(PyriteRoutineTask *task) {
    pyrite_start_task(pyrite_routine_run, task);
}

static void pyrite_join_routines(void) {
    pthread_mutex_lock(&pyrite_routine_state_lock);
    while (pyrite_routine_active_count > 0) {
        pthread_cond_wait(&pyrite_routine_state_done, &pyrite_routine_state_lock);
    }
    if (pyrite_routine_pool_started) {
        pyrite_routine_pool_stopping = 1;
        pthread_cond_broadcast(&pyrite_routine_state_work);
    }
    pthread_mutex_unlock(&pyrite_routine_state_lock);
    for (size_t i = 0; i < pyrite_routine_worker_count; i++) {
        pthread_join(pyrite_routine_workers[i], NULL);
    }
    free(pyrite_routine_workers);
    pyrite_routine_workers = NULL;
    pyrite_routine_worker_count = 0;
    pyrite_routine_pool_started = 0;
    pyrite_routine_pool_stopping = 0;
    pyrite_routine_queue_head = NULL;
    pyrite_routine_queue_tail = NULL;
}

static long pyrite_routine_set_workers(long count) {
    pthread_mutex_lock(&pyrite_routine_state_lock);
    if (count < 1) count = 1;
    if (count > 1024) count = 1024;
    if (!pyrite_routine_pool_started && pyrite_routine_active_count == 0) {
        pyrite_routine_configured_workers = (size_t)count;
    }
    long active = pyrite_routine_pool_started ? (long)pyrite_routine_worker_count : (long)pyrite_routine_configured_workers;
    pthread_mutex_unlock(&pyrite_routine_state_lock);
    return active;
}

static void pyrite_defer_close_file(const char *name, FILE *f) {
    if (!f) return;
    fclose(f);
    if (PYRITE_TRACE_DEFER) {
        fprintf(stderr, "defer: closed file %s, cleared 0 bytes\n", name);
    }
}

static FILE *pyrite_file_open(const char *path, const char *mode) {
    pyrite_set_error("");
    FILE *f = fopen(path, mode);
    if (!f) {
        snprintf(pyrite_last_error, sizeof(pyrite_last_error), "file.open failed: %s", path ? path : "");
    }
    return f;
}

static long pyrite_file_write(FILE *f, const char *data) {
    pyrite_set_error("");
    if (!f || !data) {
        pyrite_set_error("file.write failed: invalid file or data");
        return -1;
    }
    long n = (long)fprintf(f, "%s", data);
    if (n < 0) pyrite_set_errno_error("file.write failed");
    return n;
}

static long pyrite_file_flush(FILE *f) {
    pyrite_set_error("");
    if (!f) {
        pyrite_set_error("file.flush failed: invalid file");
        return -1;
    }
    long status = fflush(f);
    if (status != 0) pyrite_set_errno_error("file.flush failed");
    return status;
}

static size_t pyrite_defer_memory_cleanup(void) {
    size_t cleared = 0;
    PyriteAllocation *node = pyrite_allocations;
    while (node) {
        PyriteAllocation *next = node->next;
        cleared += node->bytes;
        free(node->ptr);
        pyrite_recycle_allocation_node(node);
        node = next;
    }
    pyrite_allocations = NULL;
    pyrite_release_cached_allocation_nodes();
    free(pyrite_temp_arena);
    pyrite_temp_arena = NULL;
    pyrite_temp_capacity = 0;
    pyrite_temp_used = 0;
    if (PYRITE_TRACE_DEFER) {
        fprintf(stderr, "defer: cleared %zu tracked heap bytes\n", cleared);
    }
    return cleared;
}

static int pyrite_random_seeded = 0;

static long pyrite_random_seed(long value) {
    srand((unsigned)value);
    pyrite_random_seeded = 1;
    return value;
}

static void pyrite_random_ensure_seeded(void) {
    if (!pyrite_random_seeded) {
        srand((unsigned)time(NULL));
        pyrite_random_seeded = 1;
    }
}

static long pyrite_random_int(long min, long max) {
    pyrite_random_ensure_seeded();
    if (max < min) return min;
    return min + (rand() % (int)(max - min + 1));
}

static double pyrite_random_float(void) {
    pyrite_random_ensure_seeded();
    return (double)rand() / (double)RAND_MAX;
}

static long pyrite_random_choice_int(PyriteList items) {
    if (items.len == 0 || !items.items) return 0;
    long idx = pyrite_random_int(0, (long)items.len - 1);
    return items.items[idx];
}

static PyriteAny pyrite_random_choice_any(PyriteAnyList items) {
    if (items.len == 0 || !items.items) return (PyriteAny){.kind = PYRITE_ANY_INT, .as.i = 0};
    long idx = pyrite_random_int(0, (long)items.len - 1);
    return pyrite_any_clone(items.items[idx]);
}

static long pyrite_time_sleep(double seconds) {
    if (seconds < 0) seconds = 0;
    time_t whole = (time_t)seconds;
    long nanos = (long)((seconds - (double)whole) * 1000000000.0);
    if (nanos < 0) nanos = 0;
    if (nanos > 999999999L) nanos = 999999999L;
    struct timespec req = {.tv_sec = whole, .tv_nsec = nanos};
    while (nanosleep(&req, &req) == -1 && errno == EINTR) {
    }
    return 0;
}

static int pyrite_regex_match(const char *pattern, const char *text) {
    pyrite_set_error("");
    regex_t re;
    int rc = regcomp(&re, pattern, REG_EXTENDED);
    if (rc != 0) {
        regerror(rc, &re, pyrite_last_error, sizeof(pyrite_last_error));
        return 0;
    }
    int ok = regexec(&re, text, 0, NULL, 0) == 0;
    regfree(&re);
    return ok;
}

static char *pyrite_file_read_all(FILE *f) {
    pyrite_set_error("");
    if (!f) {
        pyrite_set_error("file.read_all failed: invalid file");
        char *empty = pyrite_malloc(1);
        if (empty) empty[0] = '\0';
        return empty;
    }
    long start = ftell(f);
    fseek(f, 0, SEEK_END);
    long end = ftell(f);
    fseek(f, start, SEEK_SET);
    long n = end - start;
    if (n < 0) n = 0;
    char *buf = pyrite_calloc((size_t)n + 1, 1);
    if (!buf) {
        pyrite_set_error("file.read_all failed: out of memory");
        return "";
    }
    size_t got = fread(buf, 1, (size_t)n, f);
    if (got < (size_t)n && ferror(f)) pyrite_set_errno_error("file.read_all failed");
    return buf;
}

static char *pyrite_chomp(char *s) {
    if (!s) return "";
    size_t n = strlen(s);
    while (n > 0 && (s[n - 1] == '\n' || s[n - 1] == '\r')) {
        s[n - 1] = '\0';
        n--;
    }
    return s;
}
`
