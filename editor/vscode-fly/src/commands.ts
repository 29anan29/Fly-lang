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

function stripV(tag: string): string {
	return tag.startsWith('v') ? tag.slice(1) : tag;
}

export async function checkExtensionUpdate(context: vscode.ExtensionContext): Promise<void> {
	const current = String(context.extension.packageJSON.version ?? '0.0.0');
	let tag = '';
	let vsixUrl = '';
	try {
		const res = await fetch('https://api.github.com/repos/29anan29/Fly-lang/releases/latest', {
			headers: { 'User-Agent': 'fly-lang-vscode', Accept: 'application/vnd.github+json' }
		});
		if (!res.ok) {
			throw new Error(`HTTP ${res.status}`);
		}
		const rel = await res.json() as { tag_name?: string; assets?: { name: string; browser_download_url: string }[] };
		tag = stripV(rel.tag_name ?? '');
		vsixUrl = (rel.assets ?? []).find(a => a.name.endsWith('.vsix'))?.browser_download_url ?? '';
	} catch (err) {
		void vscode.window.showWarningMessage(
			`检查插件更新失败（${err instanceof Error ? err.message : String(err)}）。编译器更新请运行 \`fly update\`；插件更新请到 GitHub Releases 页面手动下载 .vsix 安装。`
		);
		return;
	}
	if (!tag || tag === current) {
		void vscode.window.showInformationMessage(`Fly 插件已是最新版本 ${current}`);
		return;
	}
	const action = await vscode.window.showInformationMessage(
		`发现 Fly 插件新版本 ${tag}（当前 ${current}）`,
		'打开下载页面'
	);
	if (action === '打开下载页面' && vsixUrl) {
		void vscode.env.openExternal(vscode.Uri.parse(vsixUrl.replace('/releases/download/', '/releases/expanded_assets/')));
	}
}
