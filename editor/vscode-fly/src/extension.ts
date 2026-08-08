import * as vscode from 'vscode';
import { initDiagnostics, checkDocument } from './diagnostics';
import { buildFile, runFile, checkForUpdates } from './commands';

export function activate(context: vscode.ExtensionContext): void {
	initDiagnostics(context);

	context.subscriptions.push(
		vscode.commands.registerCommand('fly.check', () => void activeCheck()),
		vscode.commands.registerCommand('fly.build', () => activeBuild()),
		vscode.commands.registerCommand('fly.run', () => activeRun()),
		vscode.commands.registerCommand('fly.update', () => checkForUpdates())
	);
}

function activeDocument(): vscode.TextDocument | undefined {
	const editor = vscode.window.activeTextEditor;
	return editor && editor.document.languageId === 'fly' ? editor.document : undefined;
}

function warnNoFile(): void {
	void vscode.window.showWarningMessage('请先打开一个 .fly 文件');
}

function activeCheck(): void {
	const doc = activeDocument();
	if (!doc) {
		warnNoFile();
		return;
	}
	void checkDocument(doc);
}

function activeBuild(): void {
	const doc = activeDocument();
	if (!doc) {
		warnNoFile();
		return;
	}
	buildFile(doc);
}

function activeRun(): void {
	const doc = activeDocument();
	if (!doc) {
		warnNoFile();
		return;
	}
	runFile(doc);
}

export function deactivate(): void {
}
