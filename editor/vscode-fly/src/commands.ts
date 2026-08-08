import * as vscode from 'vscode';
import { flyPath, flyProxyArgs, shellQuote } from './fly';

function runInTerminal(command: string): void {
	const terminal = vscode.window.createTerminal('Fly');
	terminal.sendText(command);
	terminal.show();
}

export function buildFile(doc: vscode.TextDocument): void {
	const outPath = doc.uri.fsPath.replace(/\.fly$/, '.py');
	runInTerminal(`${flyPath()} build -o ${shellQuote(outPath)} ${shellQuote(doc.uri.fsPath)}`);
}

export function runFile(doc: vscode.TextDocument): void {
	runInTerminal(`${flyPath()} run ${shellQuote(doc.uri.fsPath)}`);
}

export function checkForUpdates(): void {
	runInTerminal(`${flyPath()} update --check ${flyProxyArgs().join(' ')}`.trimEnd());
}
