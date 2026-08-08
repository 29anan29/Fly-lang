import * as vscode from 'vscode';
import { execFile } from 'child_process';
import { promisify } from 'util';

export interface FlyResult {
	code: number | null;
	stdout: string;
	stderr: string;
}

export function flyPath(): string {
	return vscode.workspace.getConfiguration('fly').get('path', 'fly');
}

const execFileAsync = promisify(execFile);

export async function runFly(args: string[]): Promise<FlyResult> {
	try {
		const { stdout, stderr } = await execFileAsync(flyPath(), args, { windowsHide: true });
		return { code: 0, stdout, stderr };
	} catch (err) {
		const e = err as { code?: number | string; stdout?: string; stderr?: string; message?: string };
		if (e.code === 'ENOENT') {
			vscode.window.showErrorMessage(
				`找不到 fly 编译器（${flyPath()}）。请在设置 fly.path 指定路径，或在 github.com/29anan29/Fly-lang/releases 下载`
			);
			return { code: null, stdout: '', stderr: '' };
		}
		return { code: typeof e.code === 'number' ? e.code : 1, stdout: e.stdout ?? '', stderr: e.stderr ?? e.message ?? '' };
	}
}

export function shellQuote(p: string): string {
	return `'${p.replace(/'/g, `'\\''`)}'`;
}
