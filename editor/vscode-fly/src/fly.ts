import * as vscode from 'vscode';

export function flyPath(): string {
	return vscode.workspace.getConfiguration('fly').get('path', 'fly');
}

export function flyProxyArgs(): string[] {
	const proxy = vscode.workspace.getConfiguration('fly').get<string>('proxy', '');
	return proxy ? ['--proxy', proxy] : [];
}

export function shellQuote(p: string): string {
	return `'${p.replace(/'/g, `'\\''`)}'`;
}
