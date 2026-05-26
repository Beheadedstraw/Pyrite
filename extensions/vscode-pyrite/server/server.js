const {
  CompletionItemKind,
  InsertTextFormat,
  TextDocuments,
  createConnection,
  ProposedFeatures,
  TextDocumentSyncKind
} = require("vscode-languageserver/node");
const { TextDocument } = require("vscode-languageserver-textdocument");

const connection = createConnection(ProposedFeatures.all);
const documents = new TextDocuments(TextDocument);

connection.onInitialize(() => ({
  capabilities: {
    textDocumentSync: TextDocumentSyncKind.Incremental,
    completionProvider: {
      triggerCharacters: [".", "(", " "],
      resolveProvider: false
    }
  }
}));

documents.onDidChangeContent(() => {
  // Placeholder for diagnostics once the compiler exposes parse-only checks.
});

connection.onCompletion((params) => {
  const document = documents.get(params.textDocument.uri);
  if (!document) {
    return [];
  }

  const text = document.getText();
  const offset = document.offsetAt(params.position);
  const linePrefix = text.slice(lineStartOffset(text, offset), offset);
  const dotMatch = linePrefix.match(/([A-Za-z_][A-Za-z0-9_]*(?:\[[^\]]+\])?(?:\.[A-Za-z_][A-Za-z0-9_]*(?:\[[^\]]+\])?)*|\"(?:\\.|[^\"])*\"|'(?:\\.|[^'])*'|[0-9]+(?:\.[0-9]+)?)\.$/);
  const index = buildDocumentIndex(text);

  if (dotMatch) {
    const receiver = dotMatch[1];
    if (STDLIB[receiver]) {
      return moduleCompletions(receiver);
    }
    if (index.enums.has(receiver)) {
      return enumMemberCompletions(receiver, index);
    }
    const receiverType = typeOfExpression(receiver, index);
    return memberCompletions(receiverType, index);
  }

  return dedupe([
    ...keywordCompletions(),
    ...typeCompletions(),
    ...builtinCompletions(),
    ...stdlibModuleCompletions(),
    ...snippetCompletions(),
    ...documentSymbolCompletions(index)
  ]);
});

documents.listen(connection);
connection.listen();

const STDLIB = {
  file: [
    ["open", "open(path: string, mode: string) -> file"],
    ["read_all", "read_all(handle: file) -> string"],
    ["write", "write(handle: file, data: string) -> int"],
    ["flush", "flush(handle: file) -> int"],
    ["last_error", "last_error() -> string"]
  ],
  net: [
    ["tcp", "tcp(host: string, port: int) -> socket"],
    ["udp", "udp(host: string, port: int) -> socket"],
    ["listen", "listen(host: string, port: int) -> listener"],
    ["listen_backlog", "listen_backlog(host: string, port: int, backlog: int) -> listener"],
    ["accept", "accept(server: listener) -> socket"],
    ["read", "read(sock: socket, max_bytes: int) -> string"],
    ["write", "write(sock: socket, data: string) -> int"],
    ["close", "close(sock: socket) -> int"],
    ["close_listener", "close_listener(server: listener) -> int"],
    ["last_error", "last_error() -> string"]
  ],
  http: [
    ["response", "response(status: string, content_type: string, body: string) -> string"],
    ["response_with_connection", "response_with_connection(status: string, content_type: string, body: string, connection: string) -> string"],
    ["ok_text", "ok_text(body: string) -> string"],
    ["ok_json", "ok_json(body: string) -> string"],
    ["ok_text_conn", "ok_text_conn(body: string, connection: string) -> string"],
    ["ok_json_conn", "ok_json_conn(body: string, connection: string) -> string"],
    ["json", "json(body: string) -> string"],
    ["text", "text(body: string) -> string"],
    ["json_conn", "json_conn(body: string, connection: string) -> string"],
    ["text_conn", "text_conn(body: string, connection: string) -> string"],
    ["created_json", "created_json(body: string) -> string"],
    ["accepted_json", "accepted_json(body: string) -> string"],
    ["no_content", "no_content() -> string"],
    ["not_found", "not_found() -> string"],
    ["not_found_conn", "not_found_conn(connection: string) -> string"],
    ["bad_request", "bad_request() -> string"],
    ["method_not_allowed", "method_not_allowed() -> string"],
    ["server_error", "server_error() -> string"],
    ["listen", "listen(host: string, port: int) -> listener"],
    ["listen_backlog", "listen_backlog(host: string, port: int, backlog: int) -> listener"],
    ["accept", "accept(server: listener) -> socket"],
    ["close", "close(client: socket) -> int"],
    ["close_listener", "close_listener(server: listener) -> int"],
    ["request_method", "request_method(request: string) -> string"],
    ["request_path", "request_path(request: string) -> string"],
    ["request_target", "request_target(request: string) -> string"],
    ["path", "path(request: string) -> string"],
    ["query", "query(request: string) -> string"],
    ["request_version", "request_version(request: string) -> string"],
    ["read_request", "read_request(client: socket) -> string"],
    ["should_close", "should_close(request: string) -> bool"],
    ["response_connection", "response_connection(request: string) -> string"],
    ["write_response", "write_response(client: socket, reply: string) -> int"],
    ["respond", "respond(client: socket, reply: string) -> int"],
    ["route", "route(request: string, method: string, route_path: string) -> bool"],
    ["match", "match(method: string, path: string, route_method: string, route_path: string) -> bool"],
    ["match_get", "match_get(path: string, route_path: string) -> bool"],
    ["get", "get(request: string, route_path: string) -> bool"],
    ["post", "post(request: string, route_path: string) -> bool"],
    ["put", "put(request: string, route_path: string) -> bool"],
    ["delete", "delete(request: string, route_path: string) -> bool"]
  ],
  regex: [["match", "match(pattern: string, text: string) -> bool"]],
  random: [
    ["int", "int(min: int, max: int) -> int"],
    ["seed", "seed(value: int) -> int"],
    ["float", "float() -> float"],
    ["choice", "choice(items: list[any]) -> any"]
  ],
  time: [["sleep", "sleep(seconds: float) -> int"]],
  routines: [
    ["lock", "lock(m: mux) -> int"],
    ["unlock", "unlock(m: mux) -> int"],
    ["workers", "workers(count: int) -> int"]
  ],
  strings: [
    ["strip", "strip(value: string) -> string"],
    ["lstrip", "lstrip(value: string) -> string"],
    ["rstrip", "rstrip(value: string) -> string"],
    ["upper", "upper(value: string) -> string"],
    ["lower", "lower(value: string) -> string"],
    ["len", "len(value: string) -> int"],
    ["find", "find(value: string, needle: string) -> int"],
    ["contains", "contains(value: string, needle: string) -> bool"],
    ["startswith", "startswith(value: string, prefix: string) -> bool"],
    ["endswith", "endswith(value: string, suffix: string) -> bool"],
    ["replace", "replace(value: string, old: string, replacement: string) -> string"],
    ["slice", "slice(value: string, start: int, end: int) -> string"],
    ["get", "get(value: string, index: int) -> string"],
    ["at", "at(value: string, index: int) -> string"],
    ["byte", "byte(value: string, index: int) -> int"]
  ],
  ints: [
    ["abs", "abs(value: int) -> int"],
    ["min", "min(left: int, right: int) -> int"],
    ["max", "max(left: int, right: int) -> int"],
    ["clamp", "clamp(value: int, min: int, max: int) -> int"],
    ["to_float", "to_float(value: int) -> float"],
    ["to_string", "to_string(value: int) -> string"],
    ["is_even", "is_even(value: int) -> bool"],
    ["is_odd", "is_odd(value: int) -> bool"]
  ],
  floats: [
    ["abs", "abs(value: float) -> float"],
    ["min", "min(left: float, right: float) -> float"],
    ["max", "max(left: float, right: float) -> float"],
    ["clamp", "clamp(value: float, min: float, max: float) -> float"],
    ["round", "round(value: float) -> int"],
    ["floor", "floor(value: float) -> int"],
    ["ceil", "ceil(value: float) -> int"],
    ["trunc", "trunc(value: float) -> int"],
    ["to_int", "to_int(value: float) -> int"],
    ["to_string", "to_string(value: float) -> string"]
  ],
  json: [
    ["stringify", "stringify(value: any) -> string"],
    ["stringify_list", "stringify_list(items: list[any]) -> string"],
    ["parse", "parse(value: string) -> any"],
    ["parse_array", "parse_array(value: string) -> list[any]"],
    ["get_string", "get_string(value: string, key: string) -> string"],
    ["get_int", "get_int(value: string, key: string) -> int"],
    ["get_float", "get_float(value: string, key: string) -> float"],
    ["get_bool", "get_bool(value: string, key: string) -> bool"],
    ["dumps", "dumps(value: any) -> string"],
    ["dumps_list", "dumps_list(items: list[any]) -> string"],
    ["loads", "loads(value: string) -> any"],
    ["loads_array", "loads_array(value: string) -> list[any]"]
  ],
  xml: [
    ["escape", "escape(value: string) -> string"],
    ["unescape", "unescape(value: string) -> string"],
    ["tag", "tag(name: string, text: string) -> string"],
    ["text", "text(value: string, name: string) -> string"],
    ["element", "element(name: string, text_value: string) -> string"],
    ["get_text", "get_text(value: string, name: string) -> string"]
  ],
  vectors: [
    ["add2", "add2(a: list[int], b: list[int]) -> list[int]"],
    ["sub2", "sub2(a: list[int], b: list[int]) -> list[int]"],
    ["scale2", "scale2(v: list[int], scalar: int) -> list[int]"],
    ["dot2", "dot2(a: list[int], b: list[int]) -> int"],
    ["length_sq2", "length_sq2(v: list[int]) -> int"],
    ["add3", "add3(a: list[int], b: list[int]) -> list[int]"],
    ["sub3", "sub3(a: list[int], b: list[int]) -> list[int]"],
    ["scale3", "scale3(v: list[int], scalar: int) -> list[int]"],
    ["dot3", "dot3(a: list[int], b: list[int]) -> int"],
    ["cross3", "cross3(a: list[int], b: list[int]) -> list[int]"],
    ["length_sq3", "length_sq3(v: list[int]) -> int"]
  ],
  kernel: [
    ["print", "print(value: string) -> int"],
    ["println", "println(value: string) -> int"],
    ["cls", "cls() -> int"],
    ["input", "input(prompt: string) -> string"],
    ["color", "color(value: int) -> int"],
    ["panic", "panic(message: string) -> int"],
    ["halt", "halt() -> int"],
    ["write_port", "write_port(port: int, value: int) -> int"],
    ["read_port", "read_port(port: int) -> int"],
    ["outb", "outb(port: int, value: int) -> int"],
    ["inb", "inb(port: int) -> int"],
    ["outw", "outw(port: int, value: int) -> int"],
    ["inw", "inw(port: int) -> int"],
    ["read64", "read64(address: int) -> int"],
    ["write64", "write64(address: int, value: int) -> int"],
    ["read8", "read8(address: int) -> int"],
    ["write8", "write8(address: int, value: int) -> int"],
    ["read_cr3", "read_cr3() -> int"],
    ["write_cr3", "write_cr3(value: int) -> int"],
    ["flush_page", "flush_page(address: int) -> int"],
    ["shr", "shr(value: int, bits: int) -> int"],
    ["shl", "shl(value: int, bits: int) -> int"],
    ["ptr", "ptr(value: string) -> int"],
    ["string_at", "string_at(address: int) -> string"],
    ["string_byte", "string_byte(value: string, index: int) -> int"],
    ["setup_user_mode", "setup_user_mode(kernel_cr3: int) -> int"],
    ["enter_user", "enter_user(space: int, entry: int, stack: int) -> int"]
  ]
};

const STRING_METHODS = STDLIB.strings.map(([label, detail]) => {
  if (label === "get" || label === "at") return [label, `${label}(index: int) -> string`];
  if (label === "byte") return [label, "byte(index: int) -> int"];
  return [label, detail.replace(/^[^(]+\((?:value: string,\s*)?/, `${label}(`)];
});

const BYTES_METHODS = [
  ["len", "len() -> int"],
  ["get", "get(index: int) -> int"],
  ["at", "at(index: int) -> int"],
  ["slice", "slice(start: int, end: int) -> bytes"],
  ["push", "push(value: int) -> bytes"],
  ["to_string", "to_string() -> string"]
];

const LIST_METHODS = [
  ["len", "len() -> int"],
  ["get", "get(index: int) -> item"],
  ["at", "at(index: int) -> item"],
  ["set", "set(index: int, value) -> list"],
  ["push", "push(value) -> list"],
  ["pop", "pop() -> list"],
  ["peek", "peek() -> item"]
];

const DICT_METHODS = [
  ["len", "len() -> int"],
  ["has", "has(key: string) -> bool"],
  ["get", "get(key: string) -> value"],
  ["get_string", "get_string(key: string) -> string"],
  ["get_int", "get_int(key: string) -> int"],
  ["set", "set(key: string, value) -> dict"],
  ["remove", "remove(key: string) -> dict"]
];

const SET_METHODS = [
  ["len", "len() -> int"],
  ["has", "has(value: string) -> bool"],
  ["add", "add(value: string) -> set"],
  ["remove", "remove(value: string) -> set"]
];

const STRING_BUILDER_METHODS = [
  ["write", "write(value: string) -> string_builder"],
  ["string", "string() -> string"],
  ["to_string", "to_string() -> string"],
  ["len", "len() -> int"]
];

const BYTES_BUILDER_METHODS = [
  ["write", "write(value: bytes) -> bytes_builder"],
  ["push", "push(value: int) -> bytes_builder"],
  ["bytes", "bytes() -> bytes"],
  ["to_bytes", "to_bytes() -> bytes"],
  ["len", "len() -> int"]
];

function keywordCompletions() {
  return ["import", "global", "const", "class", "enum", "def", "if", "else", "while", "for", "foreach", "switch", "match", "case", "default", "try", "except", "raise", "return", "async", "True", "False"].map((label) =>
    completion(label, CompletionItemKind.Keyword, "Pyrite keyword")
  );
}

function typeCompletions() {
  return ["int", "float", "string", "str", "bytes", "bool", "any", "list[int]", "list[any]", "list[T]", "dict", "dict[T]", "set", "string_builder", "bytes_builder", "file", "socket", "listener", "mux"].map((label) =>
    completion(label, CompletionItemKind.TypeParameter, "Pyrite type")
  );
}

function builtinCompletions() {
  return [
    ["print", "print(value)"],
    ["async", "async(helper(args))"],
    ["routine", "routine(call, optional_mux)"],
    ["mux", "mux() -> mux"],
    ["bytes", "bytes(values: list[int]) -> bytes"],
    ["dict", "dict() -> dict"],
    ["set", "set() -> set"],
    ["string_builder", "string_builder() -> string_builder"],
    ["bytes_builder", "bytes_builder() -> bytes_builder"]
  ].map(([label, detail]) => completion(label, CompletionItemKind.Function, detail));
}

function stdlibModuleCompletions() {
  return Object.keys(STDLIB).map((label) => completion(label, CompletionItemKind.Module, `import ${label}`));
}

function moduleCompletions(moduleName) {
  const members = STDLIB[moduleName];
  if (!members) return [];
  return members.map(([label, detail]) => ({
    label,
    kind: CompletionItemKind.Function,
    detail,
    insertText: `${label}($0)`,
    insertTextFormat: InsertTextFormat.Snippet
  }));
}

function enumMemberCompletions(enumName, index) {
  const enumInfo = index.enums.get(enumName);
  if (!enumInfo) return [];
  return Array.from(enumInfo.members).map((label) => completion(label, CompletionItemKind.EnumMember, `${enumName}.${label}`));
}

function memberCompletions(typeName, index) {
  if (!typeName) return [];
  if (typeName === "string") return methodCompletions(STRING_METHODS);
  if (typeName === "bytes") return methodCompletions(BYTES_METHODS);
  if (typeName === "int") return methodCompletions(STDLIB.ints);
  if (typeName === "float") return methodCompletions(STDLIB.floats);
  if (isListType(typeName)) return methodCompletions(LIST_METHODS);
  if (isDictType(typeName)) return methodCompletions(DICT_METHODS);
  if (typeName === "set") return methodCompletions(SET_METHODS);
  if (typeName === "string_builder") return methodCompletions(STRING_BUILDER_METHODS);
  if (typeName === "bytes_builder") return methodCompletions(BYTES_BUILDER_METHODS);
  if (typeName.startsWith("class:")) {
    const className = typeName.slice("class:".length);
    const classInfo = index.classes.get(className);
    if (!classInfo) return [];
    const fields = Array.from(classInfo.fields.entries()).map(([label, fieldType]) =>
      completion(label, CompletionItemKind.Field, `${label}: ${fieldType}`)
    );
    const methods = Array.from(classInfo.methods.keys()).map((label) =>
      completion(label, CompletionItemKind.Method, `${className}.${label}(...)`)
    );
    return dedupe([...fields, ...methods]);
  }
  return [];
}

function methodCompletions(methods) {
  return methods.map(([label, detail]) => ({
    label,
    kind: CompletionItemKind.Method,
    detail,
    insertText: `${label}($0)`,
    insertTextFormat: InsertTextFormat.Snippet
  }));
}

function snippetCompletions() {
  return [
    ["def main", "def main():\n    $1\n    return 0", "main function"],
    ["class", "class ${1:Name}:\n    def __init__(self, ${2:value: int}):\n        self.${3:value} = ${4:value}", "class declaration"],
    ["enum", "enum ${1:Name}:\n    ${2:ITEM}\n    ${3:OTHER} = ${4:10}", "enum declaration"],
    ["if", "if ${1:condition}:\n    $0", "if block"],
    ["while", "while ${1:condition}:\n    $0", "while loop"],
    ["switch", "switch ${1:value}:\n    case ${2:\"value\"}:\n        $3\n    default:\n        $0", "switch/case block"],
    ["match", "match ${1:value}:\n    case ${2:TokenKind.IDENT}:\n        $3\n    case _:\n        $0", "match/case block"],
    ["try", "try:\n    $1\nexcept ${2:err}:\n    print(${2:err})", "try/except block"]
  ].map(([label, insertText, detail]) => ({
    label,
    kind: CompletionItemKind.Snippet,
    detail,
    insertText,
    insertTextFormat: InsertTextFormat.Snippet
  }));
}

function documentSymbolCompletions(index) {
  const items = [];
  for (const label of index.functions) {
    items.push(completion(label, CompletionItemKind.Function, "local function"));
  }
  for (const label of index.classes.keys()) {
    items.push(completion(label, CompletionItemKind.Class, "local class"));
  }
  for (const [enumName, enumInfo] of index.enums.entries()) {
    items.push(completion(enumName, CompletionItemKind.Enum, "local enum"));
    for (const member of enumInfo.members) {
      items.push(completion(`${enumName}.${member}`, CompletionItemKind.EnumMember, "enum member"));
    }
  }
  for (const [label, typeName] of index.variables.entries()) {
    items.push(completion(label, CompletionItemKind.Variable, typeName ? `local variable: ${typeName}` : "local variable"));
  }
  for (const classInfo of index.classes.values()) {
    for (const [label, typeName] of classInfo.fields.entries()) {
      items.push(completion(label, CompletionItemKind.Field, typeName ? `class field: ${typeName}` : "class field"));
    }
  }
  return items;
}

function buildDocumentIndex(text) {
  const index = {
    variables: new Map(),
    functions: new Set(),
    classes: new Map(),
    enums: new Map()
  };

  const lines = text.split(/\r?\n/);
  let currentClass = null;
  let currentClassIndent = -1;
  let currentEnum = null;
  let currentEnumIndent = -1;
  let currentMethodParams = new Map();

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const indent = line.match(/^\s*/)[0].length;

    if (currentClass && indent <= currentClassIndent && !trimmed.startsWith("def ")) {
      currentClass = null;
      currentClassIndent = -1;
      currentMethodParams = new Map();
    }
    if (currentEnum && indent <= currentEnumIndent) {
      currentEnum = null;
      currentEnumIndent = -1;
    }

    if (currentEnum && indent > currentEnumIndent) {
      const enumMember = trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*)(?:\s*=\s*[0-9]+)?$/);
      if (enumMember) currentEnum.members.add(enumMember[1]);
      continue;
    }

    const classMatch = trimmed.match(/^class\s+([A-Za-z_][A-Za-z0-9_]*)\s*:/);
    if (classMatch) {
      currentClass = ensureClass(index, classMatch[1]);
      currentClassIndent = indent;
      currentMethodParams = new Map();
      continue;
    }

    const enumMatch = trimmed.match(/^enum\s+([A-Za-z_][A-Za-z0-9_]*)\s*:/);
    if (enumMatch) {
      currentEnum = ensureEnum(index, enumMatch[1]);
      currentEnumIndent = indent;
      continue;
    }

    const defMatch = trimmed.match(/^def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*:/);
    if (defMatch) {
      const fnName = defMatch[1];
      if (currentClass && indent > currentClassIndent) {
        currentClass.methods.set(fnName, parseParams(defMatch[2]));
      } else {
        index.functions.add(fnName);
      }
      currentMethodParams = parseParams(defMatch[2]);
      for (const [param, typeName] of currentMethodParams.entries()) {
        if (param !== "self" && typeName) index.variables.set(param, typeName);
      }
      continue;
    }

    const selfAssign = trimmed.match(/^self\.([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+)$/);
    if (selfAssign && currentClass) {
      currentClass.fields.set(selfAssign[1], inferType(selfAssign[2], index, currentMethodParams));
      continue;
    }

    const assign = trimmed.match(/^(?:const\s+|global\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?::\s*([^=]+?))?\s*=\s*(.+)$/);
    if (assign) {
      const name = assign[1];
      const annotation = normalizeType(assign[2]);
      const inferred = annotation || inferType(assign[3], index, currentMethodParams);
      index.variables.set(name, inferred);
    }
  }

  return index;
}

function ensureClass(index, name) {
  if (!index.classes.has(name)) {
    index.classes.set(name, {
      name,
      fields: new Map(),
      methods: new Map()
    });
  }
  return index.classes.get(name);
}

function ensureEnum(index, name) {
  if (!index.enums.has(name)) {
    index.enums.set(name, {
      name,
      members: new Set()
    });
  }
  return index.enums.get(name);
}

function parseParams(rawParams) {
  const params = new Map();
  for (const raw of splitArgs(rawParams)) {
    const part = raw.trim();
    if (!part) continue;
    const match = part.match(/^([A-Za-z_][A-Za-z0-9_]*)(?:\s*:\s*(.+))?$/);
    if (match) {
      params.set(match[1], normalizeType(match[2]));
    }
  }
  return params;
}

function inferType(expr, index, params = new Map()) {
  const value = expr.trim();
  if (!value) return undefined;
  if (/^b\"/.test(value)) return "bytes";
  if (/^f?\"/.test(value) || /^'/.test(value)) return "string";
  if (/^(True|False|true|false)\b/.test(value)) return "bool";
  if (/^[0-9]+\.[0-9]+$/.test(value)) return "float";
  if (/^[0-9]+$/.test(value)) return "int";
  if (isEnumMember(value, index)) return "int";
  if (/^\[/.test(value)) {
    const items = splitArgs(value.replace(/^\[/, "").replace(/\]$/, ""));
    if (!items.length) return "list[any]";
    const itemTypes = items.map((item) => inferType(item, index, params)).filter(Boolean);
    if (itemTypes.length === items.length && itemTypes.every((kind) => kind === "int")) return "list[int]";
    if (itemTypes.length === items.length && itemTypes.every((kind) => kind === itemTypes[0])) return `list[${displayType(itemTypes[0])}]`;
    return "list[any]";
  }
  const ctor = value.match(/^([A-Z][A-Za-z0-9_]*)\s*\(/);
  if (ctor && index.classes.has(ctor[1])) return `class:${ctor[1]}`;
  if (/^dict\s*\(/.test(value)) return "dict";
  if (/^set\s*\(/.test(value)) return "set";
  if (/^string_builder\s*\(/.test(value)) return "string_builder";
  if (/^bytes_builder\s*\(/.test(value)) return "bytes_builder";
  const call = value.match(/^([A-Za-z_][A-Za-z0-9_]*)(?:\.([A-Za-z_][A-Za-z0-9_]*))?\s*\(/);
  if (call) return inferCallType(call[1], call[2]);
  return typeOfExpression(value, index, params);
}

function inferCallType(base, member) {
  if (!member) {
    if (base === "bytes") return "bytes";
    if (base === "dict") return "dict";
    if (base === "set") return "set";
    if (base === "string_builder") return "string_builder";
    if (base === "bytes_builder") return "bytes_builder";
  }
  const signatures = member ? STDLIB[base] : undefined;
  if (!signatures || !member) return undefined;
  const signature = signatures.find(([label]) => label === member);
  if (!signature) return undefined;
  const match = signature[1].match(/->\s*([A-Za-z_\[\]]+)/);
  return normalizeType(match && match[1]);
}

function typeOfExpression(expr, index, params = new Map()) {
  const value = expr.trim();
  if (!value) return undefined;
  if (/^b\"/.test(value)) return "bytes";
  if (/^f?\"/.test(value) || /^'/.test(value)) return "string";
  if (/^[0-9]+\.[0-9]+$/.test(value)) return "float";
  if (/^[0-9]+$/.test(value)) return "int";
  if (isEnumMember(value, index)) return "int";
  if (params.has(value)) return params.get(value);
  if (index.variables.has(value)) return index.variables.get(value);

  const indexMatch = splitIndexedExpression(value);
  if (indexMatch) {
    const baseType = typeOfExpression(indexMatch.base, index, params);
    if (baseType === "string") return "string";
    if (baseType === "bytes") return "int";
    if (isListType(baseType)) return listElementType(baseType);
  }

  const parts = splitMemberPath(value);
  if (parts.length > 1) {
    let typeName = typeOfExpression(parts[0], index, params);
    for (const part of parts.slice(1)) {
      if (isEnumMember(`${parts[0]}.${part}`, index)) return "int";
      if (!typeName || !typeName.startsWith("class:")) return undefined;
      const classInfo = index.classes.get(typeName.slice("class:".length));
      if (!classInfo) return undefined;
      typeName = classInfo.fields.get(part);
    }
    return typeName;
  }

  return undefined;
}

function normalizeType(typeName) {
  if (!typeName) return undefined;
  const raw = typeName.replace(/\s+/g, "");
  const cleaned = raw.toLowerCase();
  const listMatch = raw.match(/^list\[([A-Za-z_][A-Za-z0-9_]*)\]$/i);
  if (listMatch) {
    const inner = normalizeType(listMatch[1]);
    return `list[${displayType(inner)}]`;
  }
  const dictMatch = raw.match(/^dict\[([A-Za-z_][A-Za-z0-9_]*)\]$/i);
  if (dictMatch) {
    const inner = normalizeType(dictMatch[1]);
    return `dict[${displayType(inner)}]`;
  }
  const aliases = {
    integer: "int",
    double: "float",
    str: "string",
    bytearray: "bytes",
    boolean: "bool",
    list: "list[int]",
    list_int: "list[int]",
    list_any: "list[any]",
    object: "object"
  };
  if (aliases[cleaned]) return aliases[cleaned];
  if (/^[A-Z][A-Za-z0-9_]*$/.test(raw)) return `class:${raw}`;
  return cleaned;
}

function isListType(typeName) {
  return /^list\[[^\]]+\]$/.test(typeName || "");
}

function listElementType(typeName) {
  const match = (typeName || "").match(/^list\[([^\]]+)\]$/);
  return match ? normalizeType(match[1]) : undefined;
}

function isDictType(typeName) {
  return typeName === "dict" || /^dict\[[^\]]+\]$/.test(typeName || "");
}

function displayType(typeName) {
  if (!typeName) return "any";
  if (typeName.startsWith("class:")) return typeName.slice("class:".length);
  return typeName;
}

function isEnumMember(value, index) {
  const match = value.match(/^([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)$/);
  if (!match) return false;
  const enumInfo = index.enums.get(match[1]);
  return !!enumInfo && enumInfo.members.has(match[2]);
}

function splitIndexedExpression(value) {
  if (!value.endsWith("]")) return undefined;
  let depth = 0;
  let quote = "";
  for (let i = value.length - 1; i >= 0; i--) {
    const ch = value[i];
    if ((ch === "\"" || ch === "'") && value[i - 1] !== "\\") {
      quote = quote === ch ? "" : quote || ch;
    }
    if (quote) continue;
    if (ch === "]") depth++;
    if (ch === "[") {
      depth--;
      if (depth === 0) return { base: value.slice(0, i).trim(), index: value.slice(i + 1, -1).trim() };
    }
  }
  return undefined;
}

function splitMemberPath(value) {
  const parts = [];
  let cur = "";
  let depth = 0;
  let quote = "";
  for (let i = 0; i < value.length; i++) {
    const ch = value[i];
    if ((ch === "\"" || ch === "'") && value[i - 1] !== "\\") {
      quote = quote === ch ? "" : quote || ch;
    }
    if (!quote) {
      if (ch === "(" || ch === "[" || ch === "{") depth++;
      if (ch === ")" || ch === "]" || ch === "}") depth--;
      if (ch === "." && depth === 0) {
        parts.push(cur.trim());
        cur = "";
        continue;
      }
    }
    cur += ch;
  }
  if (cur.trim()) parts.push(cur.trim());
  return parts;
}

function splitArgs(input) {
  const args = [];
  let cur = "";
  let depth = 0;
  let quote = "";
  for (let i = 0; i < input.length; i++) {
    const ch = input[i];
    if ((ch === "\"" || ch === "'") && input[i - 1] !== "\\") {
      quote = quote === ch ? "" : quote || ch;
    }
    if (!quote) {
      if (ch === "(" || ch === "[" || ch === "{") depth++;
      if (ch === ")" || ch === "]" || ch === "}") depth--;
      if (ch === "," && depth === 0) {
        args.push(cur.trim());
        cur = "";
        continue;
      }
    }
    cur += ch;
  }
  if (cur.trim()) args.push(cur.trim());
  return args;
}

function completion(label, kind, detail) {
  return { label, kind, detail };
}

function dedupe(items) {
  const seen = new Set();
  const out = [];
  for (const item of items) {
    const key = `${item.kind}:${item.label}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(item);
  }
  return out;
}

function lineStartOffset(text, offset) {
  const idx = text.lastIndexOf("\n", Math.max(0, offset - 1));
  return idx < 0 ? 0 : idx + 1;
}
