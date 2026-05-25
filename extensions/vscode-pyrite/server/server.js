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
  const dotMatch = linePrefix.match(/([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*|\"(?:\\.|[^\"])*\"|'(?:\\.|[^'])*'|[0-9]+(?:\.[0-9]+)?)\.$/);
  const index = buildDocumentIndex(text);

  if (dotMatch) {
    const receiver = dotMatch[1];
    if (STDLIB[receiver]) {
      return moduleCompletions(receiver);
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
    ["slice", "slice(value: string, start: int, end: int) -> string"]
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
  ]
};

function keywordCompletions() {
  return ["import", "global", "const", "class", "def", "if", "else", "while", "for", "foreach", "switch", "case", "default", "try", "except", "raise", "return", "async", "True", "False"].map((label) =>
    completion(label, CompletionItemKind.Keyword, "Pyrite keyword")
  );
}

function typeCompletions() {
  return ["int", "float", "string", "str", "bool", "any", "list[int]", "list[any]", "dict", "file", "socket", "listener", "mux"].map((label) =>
    completion(label, CompletionItemKind.TypeParameter, "Pyrite type")
  );
}

function builtinCompletions() {
  return [
    ["print", "print(value)"],
    ["async", "async(helper(args))"],
    ["routine", "routine(call, optional_mux)"],
    ["mux", "mux() -> mux"]
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

function memberCompletions(typeName, index) {
  if (!typeName) return [];
  if (typeName === "string") return methodCompletions(STDLIB.strings);
  if (typeName === "int") return methodCompletions(STDLIB.ints);
  if (typeName === "float") return methodCompletions(STDLIB.floats);
  if (typeName === "list[int]" || typeName === "list[any]") {
    return [
      completion("len", CompletionItemKind.Method, "planned list length helper"),
      completion("append", CompletionItemKind.Method, "planned list append helper"),
      completion("pop", CompletionItemKind.Method, "planned list pop helper")
    ];
  }
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
    ["if", "if ${1:condition}:\n    $0", "if block"],
    ["while", "while ${1:condition}:\n    $0", "while loop"],
    ["switch", "switch ${1:value}:\n    case ${2:\"value\"}:\n        $3\n    default:\n        $0", "switch/case block"],
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
    classes: new Map()
  };

  const lines = text.split(/\r?\n/);
  let currentClass = null;
  let currentClassIndent = -1;
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

    const classMatch = trimmed.match(/^class\s+([A-Za-z_][A-Za-z0-9_]*)\s*:/);
    if (classMatch) {
      currentClass = ensureClass(index, classMatch[1]);
      currentClassIndent = indent;
      currentMethodParams = new Map();
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

    const assign = trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*(?::\s*([^=]+?))?\s*=\s*(.+)$/);
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
  if (/^f?\"/.test(value) || /^'/.test(value)) return "string";
  if (/^(True|False|true|false)\b/.test(value)) return "bool";
  if (/^[0-9]+\.[0-9]+$/.test(value)) return "float";
  if (/^[0-9]+$/.test(value)) return "int";
  if (/^\[/.test(value)) {
    return value.includes("\"") || value.includes("'") || value.includes(".") || /\b(True|False|true|false)\b/.test(value) ? "list[any]" : "list[int]";
  }
  const ctor = value.match(/^([A-Z][A-Za-z0-9_]*)\s*\(/);
  if (ctor && index.classes.has(ctor[1])) return `class:${ctor[1]}`;
  const call = value.match(/^([A-Za-z_][A-Za-z0-9_]*)(?:\.([A-Za-z_][A-Za-z0-9_]*))?\s*\(/);
  if (call) return inferCallType(call[1], call[2]);
  return typeOfExpression(value, index, params);
}

function inferCallType(base, member) {
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
  if (/^f?\"/.test(value) || /^'/.test(value)) return "string";
  if (/^[0-9]+\.[0-9]+$/.test(value)) return "float";
  if (/^[0-9]+$/.test(value)) return "int";
  if (params.has(value)) return params.get(value);
  if (index.variables.has(value)) return index.variables.get(value);

  const parts = value.split(".");
  if (parts.length > 1) {
    let typeName = typeOfExpression(parts[0], index, params);
    for (const part of parts.slice(1)) {
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
  const cleaned = typeName.replace(/\s+/g, "").toLowerCase();
  const aliases = {
    integer: "int",
    double: "float",
    str: "string",
    boolean: "bool",
    list: "list[int]",
    list_int: "list[int]",
    list_any: "list[any]",
    object: "dict"
  };
  return aliases[cleaned] || cleaned;
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
