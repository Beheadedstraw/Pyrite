const vscode = require("vscode");
const fs = require("fs");
const path = require("path");
const {
  LanguageClient,
  TransportKind
} = require("vscode-languageclient/node");

let client;

function activate(context) {
  const output = vscode.window.createOutputChannel("Pyrite");
  client = startLanguageServer(context);

  context.subscriptions.push(
    vscode.languages.registerCompletionItemProvider(
      { language: "pyrite" },
      new PyriteCompletionProvider(),
      "."
    ),
    vscode.commands.registerCommand("pyrite.compileCurrentFile", () => compileCurrentFile({ run: false, trace: false, output })),
    vscode.commands.registerCommand("pyrite.compileCurrentFileWithTrace", () => compileCurrentFile({ run: false, trace: true, output })),
    vscode.commands.registerCommand("pyrite.runCurrentFile", () => compileCurrentFile({ run: true, trace: false, output })),
    output
  );
}

function deactivate() {
  if (!client) {
    return undefined;
  }
  return client.stop();
}

function startLanguageServer(context) {
  const serverModule = context.asAbsolutePath(path.join("server", "server.js"));
  const serverOptions = {
    run: {
      module: serverModule,
      transport: TransportKind.ipc
    },
    debug: {
      module: serverModule,
      transport: TransportKind.ipc,
      options: {
        execArgv: ["--nolazy", "--inspect=6009"]
      }
    }
  };

  const clientOptions = {
    documentSelector: [{ language: "pyrite" }],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher("**/*.pyr")
    }
  };

  const languageClient = new LanguageClient(
    "pyriteLanguageServer",
    "Pyrite Language Server",
    serverOptions,
    clientOptions
  );

  languageClient.start();
  context.subscriptions.push(languageClient);
  return languageClient;
}

async function compileCurrentFile(options) {
  const editor = vscode.window.activeTextEditor;
  if (!editor || editor.document.languageId !== "pyrite") {
    vscode.window.showWarningMessage("Open a Pyrite .pyr file first.");
    return;
  }

  const document = editor.document;
  if (document.isDirty) {
    await document.save();
  }

  const workspaceFolder = vscode.workspace.getWorkspaceFolder(document.uri);
  const workspacePath = workspaceFolder ? workspaceFolder.uri.fsPath : path.dirname(document.uri.fsPath);
  const config = vscode.workspace.getConfiguration("pyrite", document.uri);
  const compiler = resolveCompiler(config.get("compilerPath"), workspacePath);
  const outputDir = path.resolve(workspacePath, config.get("outputDirectory") || "build");
  const outputPath = path.join(outputDir, path.basename(document.uri.fsPath, path.extname(document.uri.fsPath)));

  await fs.promises.mkdir(outputDir, { recursive: true });

  const args = [];
  if (options.trace) {
    args.push("--trace-defer");
  }
  args.push(document.uri.fsPath, "-o", outputPath);

  const terminal = vscode.window.createTerminal({ name: "Pyrite" });
  terminal.show(true);
  terminal.sendText(`${shellQuote(compiler)} ${args.map(shellQuote).join(" ")}`);
  options.output.appendLine(`Compiling ${document.uri.fsPath}`);
  options.output.appendLine(`Output: ${outputPath}`);

  if (options.run) {
    terminal.sendText(shellQuote(outputPath));
  }
}

function resolveCompiler(configuredPath, workspacePath) {
  if (configuredPath && configuredPath.trim()) {
    return configuredPath.trim();
  }

  const localCompiler = path.join(workspacePath, "build", process.platform === "win32" ? "pyritec.exe" : "pyritec");
  if (fs.existsSync(localCompiler)) {
    return localCompiler;
  }

  return "pyritec";
}

function shellQuote(value) {
  if (/^[A-Za-z0-9_./:=@+-]+$/.test(value)) {
    return value;
  }
  return `'${String(value).replace(/'/g, "'\\''")}'`;
}

const METHOD_SETS = {
  string: [
    ["strip", "strip() -> string"],
    ["lstrip", "lstrip() -> string"],
    ["rstrip", "rstrip() -> string"],
    ["upper", "upper() -> string"],
    ["lower", "lower() -> string"],
    ["len", "len() -> int"],
    ["find", "find(needle: string) -> int"],
    ["contains", "contains(needle: string) -> bool"],
    ["startswith", "startswith(prefix: string) -> bool"],
    ["starts_with", "starts_with(prefix: string) -> bool"],
    ["endswith", "endswith(suffix: string) -> bool"],
    ["ends_with", "ends_with(suffix: string) -> bool"],
    ["replace", "replace(old: string, replacement: string) -> string"],
    ["slice", "slice(start: int, end: int) -> string"],
    ["get", "get(index: int) -> string"],
    ["at", "at(index: int) -> string"],
    ["byte", "byte(index: int) -> int"]
  ],
  bytes: [
    ["len", "len() -> int"],
    ["get", "get(index: int) -> int"],
    ["at", "at(index: int) -> int"],
    ["slice", "slice(start: int, end: int) -> bytes"],
    ["push", "push(value: int) -> bytes"],
    ["to_string", "to_string() -> string"]
  ],
  list: [
    ["len", "len() -> int"],
    ["get", "get(index: int) -> item"],
    ["at", "at(index: int) -> item"],
    ["set", "set(index: int, value) -> list"],
    ["push", "push(value) -> list"],
    ["pop", "pop() -> list"],
    ["peek", "peek() -> item"]
  ],
  dict: [
    ["len", "len() -> int"],
    ["has", "has(key: string) -> bool"],
    ["get", "get(key: string) -> value"],
    ["get_string", "get_string(key: string) -> string"],
    ["get_int", "get_int(key: string) -> int"],
    ["set", "set(key: string, value) -> dict"],
    ["remove", "remove(key: string) -> dict"]
  ],
  set: [
    ["len", "len() -> int"],
    ["has", "has(value: string) -> bool"],
    ["add", "add(value: string) -> set"],
    ["remove", "remove(value: string) -> set"]
  ],
  string_builder: [
    ["write", "write(value: string) -> string_builder"],
    ["string", "string() -> string"],
    ["to_string", "to_string() -> string"],
    ["len", "len() -> int"]
  ],
  bytes_builder: [
    ["write", "write(value: bytes) -> bytes_builder"],
    ["push", "push(value: int) -> bytes_builder"],
    ["bytes", "bytes() -> bytes"],
    ["to_bytes", "to_bytes() -> bytes"],
    ["len", "len() -> int"]
  ]
};

class PyriteCompletionProvider {
  provideCompletionItems(document, position) {
    const linePrefix = document.lineAt(position).text.slice(0, position.character);
    const dotMatch = linePrefix.match(/([A-Za-z_][A-Za-z0-9_]*(?:\[[^\]]+\])?(?:\.[A-Za-z_][A-Za-z0-9_]*(?:\[[^\]]+\])?)*|"(?:\\.|[^"])*"|'(?:\\.|[^'])*'|[0-9]+(?:\.[0-9]+)?)\.$/);
    if (!dotMatch) {
      return undefined;
    }

    const index = buildCompletionIndex(document.getText());
    const receiver = dotMatch[1];
    const typeName = inferCompletionType(receiver, index);
    return completionItemsForType(typeName, index);
  }
}

function completionItemsForType(typeName, index) {
  if (!typeName) return [];
  if (typeName === "string") return methodItems(METHOD_SETS.string);
  if (typeName === "bytes") return methodItems(METHOD_SETS.bytes);
  if (typeName === "set") return methodItems(METHOD_SETS.set);
  if (typeName === "string_builder") return methodItems(METHOD_SETS.string_builder);
  if (typeName === "bytes_builder") return methodItems(METHOD_SETS.bytes_builder);
  if (isCompletionList(typeName)) return methodItems(METHOD_SETS.list);
  if (isCompletionDict(typeName)) return methodItems(METHOD_SETS.dict);
  if (typeName.startsWith("class:")) {
    const className = typeName.slice("class:".length);
    const info = index.classes.get(className);
    if (!info) return [];
    return [
      ...Array.from(info.fields.entries()).map(([label, detail]) => completionItem(label, vscode.CompletionItemKind.Field, detail)),
      ...Array.from(info.methods.keys()).map((label) => completionItem(label, vscode.CompletionItemKind.Method, `${className}.${label}(...)`))
    ];
  }
  return [];
}

function methodItems(methods) {
  return methods.map(([label, detail]) => completionItem(label, vscode.CompletionItemKind.Method, detail));
}

function completionItem(label, kind, detail) {
  const item = new vscode.CompletionItem(label, kind);
  item.detail = detail;
  return item;
}

function buildCompletionIndex(text) {
  const index = {
    variables: new Map(),
    classes: new Map()
  };
  const lines = text.split(/\r?\n/);
  let currentClass = null;
  let currentClassIndent = -1;
  let params = new Map();

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const indent = line.match(/^\s*/)[0].length;

    if (currentClass && indent <= currentClassIndent && !trimmed.startsWith("def ")) {
      currentClass = null;
      currentClassIndent = -1;
      params = new Map();
    }

    const classMatch = trimmed.match(/^class\s+([A-Za-z_][A-Za-z0-9_]*)\s*:/);
    if (classMatch) {
      currentClass = ensureCompletionClass(index, classMatch[1]);
      currentClassIndent = indent;
      params = new Map();
      continue;
    }

    const defMatch = trimmed.match(/^def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*:/);
    if (defMatch) {
      params = parseCompletionParams(defMatch[2]);
      if (currentClass && indent > currentClassIndent) {
        currentClass.methods.set(defMatch[1], params);
      }
      for (const [name, typeName] of params.entries()) {
        if (name !== "self" && typeName) index.variables.set(name, typeName);
      }
      continue;
    }

    const selfAssign = trimmed.match(/^self\.([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+)$/);
    if (selfAssign && currentClass) {
      currentClass.fields.set(selfAssign[1], inferCompletionType(selfAssign[2], index, params));
      continue;
    }

    const assign = trimmed.match(/^(?:const\s+|global\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?::\s*([^=]+?))?\s*=\s*(.+)$/);
    if (assign) {
      const annotation = normalizeCompletionType(assign[2]);
      index.variables.set(assign[1], annotation || inferCompletionType(assign[3], index, params));
    }
  }
  return index;
}

function ensureCompletionClass(index, name) {
  if (!index.classes.has(name)) {
    index.classes.set(name, { fields: new Map(), methods: new Map() });
  }
  return index.classes.get(name);
}

function parseCompletionParams(raw) {
  const params = new Map();
  for (const part of splitCompletionArgs(raw)) {
    const match = part.match(/^([A-Za-z_][A-Za-z0-9_]*)(?:\s*:\s*(.+))?$/);
    if (match) params.set(match[1], normalizeCompletionType(match[2]));
  }
  return params;
}

function inferCompletionType(expr, index, params = new Map()) {
  const value = expr.trim();
  if (!value) return undefined;
  if (/^b"/.test(value)) return "bytes";
  if (/^f?"/.test(value) || /^'/.test(value)) return "string";
  if (/^(True|False|true|false)\b/.test(value)) return "bool";
  if (/^[0-9]+\.[0-9]+$/.test(value)) return "float";
  if (/^[0-9]+$/.test(value)) return "int";
  if (params.has(value)) return params.get(value);
  if (index.variables.has(value)) return index.variables.get(value);

  const indexed = splitCompletionIndexed(value);
  if (indexed) {
    const baseType = inferCompletionType(indexed.base, index, params);
    if (baseType === "string") return "string";
    if (baseType === "bytes") return "int";
    if (isCompletionList(baseType)) return listCompletionElement(baseType);
  }

  const ctor = value.match(/^([A-Z][A-Za-z0-9_]*)\s*\(/);
  if (ctor && index.classes.has(ctor[1])) return `class:${ctor[1]}`;
  if (/^dict\s*\(/.test(value)) return "dict";
  if (/^set\s*\(/.test(value)) return "set";
  if (/^string_builder\s*\(/.test(value)) return "string_builder";
  if (/^bytes_builder\s*\(/.test(value)) return "bytes_builder";

  const parts = splitCompletionMemberPath(value);
  if (parts.length > 1) {
    let typeName = inferCompletionType(parts[0], index, params);
    for (const field of parts.slice(1)) {
      if (!typeName || !typeName.startsWith("class:")) return undefined;
      const info = index.classes.get(typeName.slice("class:".length));
      if (!info) return undefined;
      typeName = info.fields.get(field);
    }
    return typeName;
  }

  return undefined;
}

function normalizeCompletionType(typeName) {
  if (!typeName) return undefined;
  const raw = typeName.replace(/\s+/g, "");
  const lower = raw.toLowerCase();
  const list = raw.match(/^list\[([A-Za-z_][A-Za-z0-9_]*)\]$/i);
  if (list) return `list[${displayCompletionType(normalizeCompletionType(list[1]))}]`;
  const dict = raw.match(/^dict\[([A-Za-z_][A-Za-z0-9_]*)\]$/i);
  if (dict) return `dict[${displayCompletionType(normalizeCompletionType(dict[1]))}]`;
  const aliases = {
    integer: "int",
    double: "float",
    str: "string",
    bytearray: "bytes",
    boolean: "bool",
    list: "list[int]",
    list_int: "list[int]",
    list_any: "list[any]"
  };
  if (aliases[lower]) return aliases[lower];
  if (/^[A-Z][A-Za-z0-9_]*$/.test(raw)) return `class:${raw}`;
  return lower;
}

function isCompletionList(typeName) {
  return /^list\[[^\]]+\]$/.test(typeName || "");
}

function isCompletionDict(typeName) {
  return typeName === "dict" || /^dict\[[^\]]+\]$/.test(typeName || "");
}

function listCompletionElement(typeName) {
  const match = (typeName || "").match(/^list\[([^\]]+)\]$/);
  return match ? normalizeCompletionType(match[1]) : undefined;
}

function displayCompletionType(typeName) {
  return typeName && typeName.startsWith("class:") ? typeName.slice("class:".length) : (typeName || "any");
}

function splitCompletionIndexed(value) {
  const match = value.match(/^(.+)\[[^\]]+\]$/);
  return match ? { base: match[1].trim() } : undefined;
}

function splitCompletionMemberPath(value) {
  const parts = [];
  let cur = "";
  let depth = 0;
  for (const ch of value) {
    if (ch === "(" || ch === "[" || ch === "{") depth++;
    if (ch === ")" || ch === "]" || ch === "}") depth--;
    if (ch === "." && depth === 0) {
      parts.push(cur.trim());
      cur = "";
      continue;
    }
    cur += ch;
  }
  if (cur.trim()) parts.push(cur.trim());
  return parts;
}

function splitCompletionArgs(input) {
  const args = [];
  let cur = "";
  let depth = 0;
  for (const ch of input || "") {
    if (ch === "(" || ch === "[" || ch === "{") depth++;
    if (ch === ")" || ch === "]" || ch === "}") depth--;
    if (ch === "," && depth === 0) {
      if (cur.trim()) args.push(cur.trim());
      cur = "";
      continue;
    }
    cur += ch;
  }
  if (cur.trim()) args.push(cur.trim());
  return args;
}

module.exports = {
  activate,
  deactivate
};
