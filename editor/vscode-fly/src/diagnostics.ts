import * as vscode from 'vscode';
import { runFly } from './fly';

const diag = vscode.languages.createDiagnosticCollection('fly');

const errorRe = /^error: (.+?\.fly):(\d+):(\d+): (.+)$/;

export function initDiagnostics(context: vscode.ExtensionContext): void {
	context.subscriptions.push(
		vscode.workspace.onDidSaveTextDocument(doc => {
			if (doc.languageId === 'fly') {
				void checkDocument(doc);
			}
		}),
		vscode.workspace.onDidOpenTextDocument(doc => {
			if (doc.languageId === 'fly') {
				void checkDocument(doc);
			}
		}),
		diag
	);
}

export async function checkDocument(doc: vscode.TextDocument): Promise<void> {
	const conf = vscode.workspace.getConfiguration('fly');
	if (!conf.get('checkOnSave', true)) {
		return;
	}
	const result = await runFly(['check', doc.uri.fsPath]);
	if (result.code === null) {
		return;
	}
	const items: vscode.Diagnostic[] = [];
	for (const line of result.stderr.split(/\r?\n/)) {
		const m = errorRe.exec(line);
		if (!m || m[1] !== doc.uri.fsPath) {
			continue;
		}
		const lineNo = Number(m[2]) - 1;
		const colNo = Number(m[3]) - 1;
		items.push(
			new vscode.Diagnostic(
				new vscode.Range(lineNo, colNo, lineNo, colNo + 1),
				m[4],
				vscode.DiagnosticSeverity.Error
			)
		);
	}
	diag.set(doc.uri, items);
}
