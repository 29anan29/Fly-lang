import * as vscode from 'vscode';
import { runFly, shellQuote } from './fly';

export function buildFile(doc: vscode.TextDocument): void {
	const outPath = doc.uri.fsPath.replace(/\.fly$/, '.py');
	void runFly(['build', '-o', outPath, doc.uri.fsPath]).then(result => {
		if (result.code === null) {
			return;
		}
		if (result.code !== 0) {
			vscode.window.showErrorMessage(result.stderr.trim() || '转译失败');
			return;
		}
		void vscode.window
			.showInformationMessage('转译成功', '打开输出')
			.then(choice => {
				if (choice === '打开输出') {
					void vscode.workspace
						.openTextDocument(outPath)
						.then(d => void vscode.window.showTextDocument(d));
				}
			});
	});
}

export function runFile(doc: vscode.TextDocument): void {
	const terminal = vscode.window.createTerminal('Fly Run');
	terminal.sendText(`fly run ${shellQuote(doc.uri.fsPath)}`);
	terminal.show();
}

export function checkForUpdates(): void {
	const conf = vscode.workspace.getConfiguration('fly');
	const proxy = conf.get<string>('proxy', '');
	const args = ['update', '--check'];
	if (proxy) {
		args.push('--proxy', proxy);
	}
	void runFly(args).then(result => {
		if (result.code === null) {
			return;
		}
		if (result.code === 2) {
			const latest = result.stdout.trim() || '新版本';
			void vscode.window
				.showInformationMessage(`${latest}，是否立即更新？`, '更新')
				.then(choice => {
					if (choice === '更新') {
						void runFly(['update', '--force', ...(proxy ? ['--proxy', proxy] : [])]).then(r => {
							vscode.window.showInformationMessage(
								r.code === 0 ? '更新成功，请重启终端' : (r.stderr.trim() || '更新失败')
							);
						});
					}
				});
			return;
		}
		if (result.code === 0) {
			vscode.window.showInformationMessage('已是最新版本');
		} else {
			vscode.window.showErrorMessage(result.stderr.trim() || '检查更新失败');
		}
	});
}
