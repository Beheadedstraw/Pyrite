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
    documentSelector: [{ scheme: "file", language: "pyrite" }],
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

module.exports = {
  activate,
  deactivate
};
