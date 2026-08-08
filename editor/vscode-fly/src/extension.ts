import * as vscode from 'vscode';
import {
	LanguageClient,
	LanguageClientOptions,
	ServerOptions,
	TransportKind
} from 'vscode-languageclient/node';
import { buildFile, runFile, checkForUpdates, checkExtensionUpdate } from './commands';
import { flyPath } from './fly';

let client: LanguageClient | undefined;

export function activate(context: vscode.ExtensionContext): void {
	context.subscriptions.push(
		vscode.commands.registerCommand('fly.check', () => void forceCheck()),
		vscode.commands.registerCommand('fly.build', () => activeBuild()),
		vscode.commands.registerCommand('fly.run', () => activeRun()),
		vscode.commands.registerCommand('fly.update', () => checkForUpdates()),
		vscode.commands.registerCommand('fly.updateExtension', () => void checkExtensionUpdate(context))
	);

	const serverOptions: ServerOptions = {
		command: flyPath(),
		args: ['lsp'],
		transport: TransportKind.stdio
	};

	const clientOptions: LanguageClientOptions = {
		documentSelector: [{ scheme: 'file', language: 'fly' }],
		initializationOptions: {},
		outputChannel: vscode.window.createOutputChannel('Fly Language', { log: true })
	};

	client = new LanguageClient('fly-lsp', 'Fly Language', serverOptions, clientOptions);
	void client.start().then(
		() => { /* started */ },
		err => { void vscode.window.showErrorMessage(`Fly LSP 启动失败: ${err.message ?? err}`); }
	);
	context.subscriptions.push({ dispose: () => { client = undefined; } });
}

async function forceCheck(): Promise<void> {
	const doc = activeDocument();
	if (!doc) {
		void vscode.window.showWarningMessage('请先打开一个 .fly 文件');
		return;
	}
	if (!client) {
		return;
	}
	await client.sendNotification('fly/forceCheck', { textDocument: { uri: doc.uri.toString() } });
}

function activeDocument(): vscode.TextDocument | undefined {
	const editor = vscode.window.activeTextEditor;
	return editor && editor.document.languageId === 'fly' ? editor.document : undefined;
}

function activeBuild(): void {
	const doc = activeDocument();
	if (!doc) {
		void vscode.window.showWarningMessage('请先打开一个 .fly 文件');
		return;
	}
	buildFile(doc);
}

function activeRun(): void {
	const doc = activeDocument();
	if (!doc) {
		void vscode.window.showWarningMessage('请先打开一个 .fly 文件');
		return;
	}
	runFile(doc);
}

export function deactivate(): Thenable<void> | undefined {
	return client?.stop();
}
