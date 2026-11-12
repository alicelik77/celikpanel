import { useState } from 'react';
import {
    Folder, File, ChevronRight, ChevronUp, RefreshCw,
    Trash2, Edit, Download, Upload, X,
    FolderPlus, FilePlus, AlertTriangle,
} from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { formatDateTime } from '../lib/dateFormat';
import { Button, KnownEmpty, RemoteGate, inputClass } from './ui';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { decodeListIn, useRemote } from '../lib/remote';
import { saveDownload } from '../lib/download';
import { confirmAction } from '../lib/confirmAction';
import { answerWasLost } from '../lib/requestIdentity';
import { useSaveRun } from './kit/save/saveRun';
import { useLeaveGuard } from './kit/save/leaveGuard';
import { ChangedMark, SaveBar } from './kit/save/SaveBar';

interface FileItem {
    name: string;
    path: string;
    is_dir: boolean;
    size: number;
    permissions: string;
    mod_time: string;
}

const decodeFiles = (raw: unknown) => decodeListIn<FileItem>(raw, 'files');

interface DomainFileManagerProps {
    domainId: number;
    domainName: string;
    readOnly?: boolean;
}

// Real file manager over the agent's file API: browse, edit small text files
// inline, upload, create, delete, download. Fully i18n'd; the create dialog is
// an inline panel (matching the DNS add-form pattern), not a modal.
//
// Agent'ın dosya API'si üzerinde gerçek dosya yöneticisi: gezin, küçük metin
// dosyalarını yerinde düzenle, yükle, oluştur, sil, indir. Tamamen i18n'li;
// oluşturma diyaloğu modal değil, satır içi bir paneldir (DNS ekleme-formu
// kalıbıyla uyumlu).
export function DomainFileManager({ domainId, readOnly = false }: DomainFileManagerProps) {
    const { t, locale } = useI18n();
    const [currentPath, setCurrentPath] = useState('/');
    // One read per folder. Another folder is another address, so the rows of
    // the folder just left are never drawn under the new path, and a folder
    // that could not be read is not drawn as an empty one.
    // Klasör başına bir okuma. Başka klasör başka adrestir; az önce çıkılan
    // klasörün satırları yeni yolun altında çizilmez ve okunamayan klasör boş
    // klasör gibi gösterilmez.
    const listing = useRemote(
        `/api/v1/domains/${domainId}/files?path=${encodeURIComponent(currentPath)}`,
        decodeFiles,
    );
    const loadFiles = () => void listing.retry();
    // Creating, uploading and deleting act on the folder as the server last
    // listed it, not on one being read again or on an earlier answer.
    // Oluşturma, yükleme ve silme, sunucunun son listelediği klasör üzerinde
    // yapılır.
    const folderKnown = listing.remote.state === 'known' && !listing.reading;

    // The open file and the text it was opened with.
    // Açık dosya ve açıldığı metin.
    const [editing, setEditing] = useState<{ path: string; text: string } | null>(null);

    const [createType, setCreateType] = useState<'file' | 'folder' | null>(null);
    const [newName, setNewName] = useState('');

    const [uploading, setUploading] = useState(false);

    // A refusal carries the server's reason. A dropped connection is not a
    // refusal: whether the change was made is not known, so the folder is read
    // again and nothing is sent a second time.
    // Ret, sunucunun nedenini taşır. Kopan bağlantı ret değildir: değişikliğin
    // yapılıp yapılmadığı bilinmez; klasör yeniden okunur, hiçbir şey ikinci
    // kez gönderilmez.
    // post sends one file action; null when no answer arrived.
    // post tek bir dosya eylemi gönderir; yanıt gelmezse null.
    const post = async (path: string, body: Record<string, unknown>): Promise<Response | null> => {
        try {
            return await fetch(`/api/v1/domains/${domainId}/files?path=${encodeURIComponent(path)}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });
        } catch {
            return null;
        }
    };
    const act = async (path: string, body: Record<string, unknown>): Promise<Response | null> => {
        const res = await post(path, body);
        if (!res) {
            showToast('error', t('common.resultUnknown'));
            loadFiles();
            return null;
        }
        if (res.ok) return res;
        showToast('error', fileErrorText(await readApiError(res)));
        return null;
    };
    // The file manager's own coded refusals (a path that does not exist, the
    // web folder gone, a name taken) in this screen's words; any other
    // refusal through the shared catalogue.
    // Dosya yöneticisinin kendi kodlu retleri bu ekranın sözleriyle.
    const fileErrorText = (err: ApiError, fallback?: string) => {
        switch (err.code) {
        case 'FILE_NOT_FOUND': return t('files.notFound');
        case 'FILE_ROOT_MISSING': return t('files.rootMissing');
        case 'FILE_EXISTS': return t('files.nameTaken');
        case 'FILE_REQUEST_INVALID': return t('files.requestInvalid');
        default: return fallback ?? apiErrorText(err, t);
        }
    };

    const navigateTo = (path: string) => {
        setCurrentPath(path);
        setCreateType(null);
    };

    const goUp = () => {
        if (currentPath === '/') return;
        navigateTo(currentPath.split('/').slice(0, -1).join('/') || '/');
    };

    const openFile = async (file: FileItem) => {
        if (file.is_dir) {
            navigateTo(file.path);
            return;
        }
        // The file-content API is a POST action and therefore intentionally
        // requires manage access. View access still has a safe GET download.
        if (readOnly) {
            await downloadFile(file);
            return;
        }
        if (file.size > 1024 * 1024) {
            showToast('error', t('files.tooLarge'));
            return;
        }
        const res = await act(file.path, { action: 'read' });
        if (!res) return;
        let data: { is_binary?: boolean; content?: unknown };
        try {
            data = await res.json();
        } catch {
            showToast('error', t('files.contentUnknown'));
            return;
        }
        if (data.is_binary) {
            showToast('error', t('files.binary'));
            return;
        }
        // An editor opened on anything but the file's own text would save
        // that over the file.
        // Dosyanın kendi metninden başka bir şeyle açılan düzenleyici, onu
        // dosyanın üzerine kaydederdi.
        if (typeof data.content !== 'string') {
            showToast('error', t('files.contentUnknown'));
            return;
        }
        setEditing({ path: file.path, text: data.content });
    };

    const createItem = async () => {
        if (readOnly || !folderKnown || !newName.trim() || !createType) return;
        const path = currentPath === '/' ? `/${newName}` : `${currentPath}/${newName}`;
        if (!(await act(path, { action: 'create', is_dir: createType === 'folder' }))) return;
        showToast('success', t('files.created'));
        setCreateType(null);
        setNewName('');
        loadFiles();
    };

    const deleteItem = async (file: FileItem) => {
        if (readOnly || !folderKnown) return;
        await confirmAction({ message: t('files.deleteConfirm', { name: file.name }), confirmLabel: t('common.delete'), tone: 'danger',
            action: async () => {
                if (!(await act(file.path, { action: 'delete' }))) return;
                showToast('success', t('files.deleted'));
                loadFiles();
            },
        });
    };

    // The download is read here first, so a refusal (a file renamed, moved
    // or deleted since the folder was listed) is shown in this screen's words
    // instead of opening a tab with the server's JSON answer (30 Oct 2026).
    // A missing entry also reads the folder again. Only a file that arrived
    // is handed to the browser to save.
    // İndirme önce burada okunur; ret, sunucunun JSON yanıtını yeni sekmede
    // açmak yerine bu ekranın sözleriyle gösterilir.
    const downloadFile = async (file: FileItem) => {
        const err = await saveDownload(`/api/v1/domains/${domainId}/files/download?path=${encodeURIComponent(file.path)}`, file.name);
        if (!err) return;
        showToast('error', fileErrorText(err));
        if (err.code === 'FILE_NOT_FOUND') loadFiles();
    };

    const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
        if (readOnly || !folderKnown) return;
        const file = e.target.files?.[0];
        if (!file) return;
        setUploading(true);
        const folder = currentPath;
        const reader = new FileReader();
        reader.onload = async () => {
            try {
                const base64 = (reader.result as string).split(',')[1];
                if (!(await act(folder, { action: 'upload', file_name: file.name, content: base64 }))) return;
                showToast('success', t('files.uploaded'));
                loadFiles();
            } finally {
                setUploading(false);
            }
        };
        reader.onerror = () => {
            setUploading(false);
            showToast('error', t('files.uploadUnreadable', { name: file.name }));
        };
        reader.readAsDataURL(file);
        e.target.value = '';
    };

    // Inline text editor takes over the whole panel while a file is open.
    // Bir dosya açıkken satır içi metin düzenleyici panelin tamamını kaplar.
    if (editing && !readOnly) {
        return (
            <FileEditor
                key={editing.path}
                path={editing.path}
                opened={editing.text}
                post={post}
                refusalText={fileErrorText}
                onSaved={loadFiles}
                onClose={() => setEditing(null)}
            />
        );
    }

    return (
        <div>
            {/* Toolbar: breadcrumb + actions */}
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-1">
                    <button
                        onClick={goUp}
                        disabled={currentPath === '/'}
                        title={t('files.up')}
                        className="rounded-md p-1.5 text-fg-muted hover:bg-surface-2 hover:text-fg disabled:opacity-40"
                    >
                        <ChevronUp className="h-4 w-4" />
                    </button>
                    <div className="flex min-w-0 items-center overflow-x-auto text-sm text-fg-muted">
                        <button onClick={() => navigateTo('/')} className="font-mono text-primary hover:underline">~</button>
                        {currentPath.split('/').filter(Boolean).map((part, i, arr) => (
                            <span key={i} className="flex items-center whitespace-nowrap">
                                <ChevronRight className="mx-0.5 h-4 w-4 shrink-0 text-fg-subtle" />
                                <button onClick={() => navigateTo('/' + arr.slice(0, i + 1).join('/'))} className="hover:text-fg">
                                    {part}
                                </button>
                            </span>
                        ))}
                    </div>
                </div>
                <div className="flex items-center gap-1">
                    {!readOnly && (
                        <>
                            <ToolButton icon={FolderPlus} title={t('files.newFolder')} disabled={!folderKnown} onClick={() => { setCreateType('folder'); setNewName(''); }} />
                            <ToolButton icon={FilePlus} title={t('files.newFile')} disabled={!folderKnown} onClick={() => { setCreateType('file'); setNewName(''); }} />
                            <label
                                title={t('files.upload')}
                                className={`rounded-md p-1.5 text-fg-muted ${folderKnown && !uploading ? 'cursor-pointer hover:bg-surface-2 hover:text-fg' : 'opacity-40'}`}
                            >
                                <Upload className="h-4 w-4" />
                                <input type="file" className="hidden" aria-label={t('files.upload')} onChange={handleUpload} disabled={uploading || !folderKnown} />
                            </label>
                        </>
                    )}
                    <ToolButton icon={RefreshCw} title={t('files.refresh')} onClick={loadFiles} spin={listing.reading} />
                </div>
            </div>

            {/* Inline create panel */}
            {!readOnly && createType && (
                <div className="mb-4 rounded-xl border border-border bg-surface-2/50 p-4">
                    <h4 className="mb-3 text-sm font-semibold text-fg">
                        {createType === 'folder' ? t('files.createFolderTitle') : t('files.createFileTitle')}
                    </h4>
                    <div className="flex flex-wrap items-center gap-2">
                        <input
                            type="text"
                            value={newName}
                            onChange={(e) => setNewName(e.target.value)}
                            onKeyDown={(e) => e.key === 'Enter' && createItem()}
                            placeholder={createType === 'folder' ? 'folder-name' : 'filename.txt'}
                            className={`${inputClass} max-w-xs font-mono`}
                            autoFocus
                        />
                        <Button variant="primary" onClick={createItem} disabled={!folderKnown || !newName.trim()}>
                            {t('files.create')}
                        </Button>
                        <Button onClick={() => { setCreateType(null); setNewName(''); }}>{t('files.cancel')}</Button>
                    </div>
                </div>
            )}

            {/* A folder that does not exist says so and offers the way back
                to the web folder; it is not shown as an unreadable one.
                Olmayan klasör bunu söyler ve web klasörüne dönüş yolunu sunar. */}
            {listing.remote.state === 'unknown' && listing.remote.reason.code === 'FILE_NOT_FOUND' && currentPath !== '/' && (
                <div className="mb-3">
                    <Button onClick={() => setCurrentPath('/')}>{t('files.backToRoot')}</Button>
                </div>
            )}
            {/* File table */}
            <RemoteGate
                remote={listing.remote}
                checking={t('files.checking')}
                failed={listing.remote.state === 'unknown'
                    ? fileErrorText(listing.remote.reason, t('files.unknown'))
                    : t('files.unknown')}
                onRetry={loadFiles}
                busy={listing.reading}
                className="py-3"
            >
                {(shown) => (shown.value.length === 0 ? (
                    <KnownEmpty of={shown} icon={Folder} title={t('files.empty')} />
                ) : (
                    <div className="overflow-x-auto rounded-xl border border-border">
                        <table className="w-full text-sm">
                            <thead>
                                <tr className="border-b border-border bg-surface-2/50 text-left text-xs font-semibold text-fg-muted">
                                    <th className="px-4 py-2.5">{t('files.col.name')}</th>
                                    <th className="w-24 px-4 py-2.5">{t('files.col.size')}</th>
                                    <th className="w-28 px-4 py-2.5">{t('files.col.perms')}</th>
                                    <th className="w-44 px-4 py-2.5">{t('files.col.modified')}</th>
                                    <th className="w-28 px-4 py-2.5" />
                                </tr>
                            </thead>
                            <tbody>
                                {shown.value.map((file) => (
                                    <tr
                                        key={file.path}
                                        onDoubleClick={() => openFile(file)}
                                        className="cursor-pointer border-b border-border last:border-0 hover:bg-surface-2/60"
                                    >
                                        <td className="px-4 py-2.5">
                                            <button onClick={() => openFile(file)} className="flex items-center gap-2 text-left">
                                                {file.is_dir ? (
                                                    <Folder className="h-4 w-4 shrink-0 text-primary" />
                                                ) : (
                                                    <File className="h-4 w-4 shrink-0 text-fg-subtle" />
                                                )}
                                                <span className="text-fg">{file.name}</span>
                                            </button>
                                        </td>
                                        <td className="px-4 py-2.5 text-fg-muted">{file.is_dir ? '—' : fmtSize(file.size)}</td>
                                        <td className="px-4 py-2.5 font-mono text-xs text-fg-muted">{file.permissions}</td>
                                        <td className="px-4 py-2.5 text-fg-muted">{fmtDate(file.mod_time, locale)}</td>
                                        <td className="px-4 py-2.5">
                                            <div className="flex items-center justify-end gap-0.5">
                                                {!file.is_dir && (
                                                    <>
                                                        {!readOnly && <ToolButton icon={Edit} title={t('files.edit')} onClick={() => openFile(file)} small />}
                                                        <ToolButton icon={Download} title={t('files.download')} onClick={() => downloadFile(file)} small />
                                                    </>
                                                )}
                                                {!readOnly && <ToolButton icon={Trash2} title={t('files.delete')} disabled={!folderKnown} onClick={() => deleteItem(file)} small danger />}
                                            </div>
                                        </td>
                                    </tr>
                ))}
                        </tbody>
                    </table>
                </div>
            ))}
            </RemoteGate>
        </div>
    );
}

// The text editor of one file (alpha.132; owner, 8 Nov 2026: nothing is saved
// without Save, unsaved changes show, and they are not dropped unseen).
//
//   - Save is offered only while the text differs from the file as it was
//     opened (or last saved); the bar says so and the name carries the mark.
//   - Closing with unsaved text asks in the panel's dialogue; leaving the page
//     asks through the shared leave guard.
//   - The file is never overwritten unseen: before writing, the file is read
//     again. When it changed on the server since it was opened, nothing is
//     written; the editor says so and the person chooses to open the server's
//     text (their changes are dropped) or to write theirs over it (asked).
//     The write itself carries no version, so a change made in the moment
//     between that read and the write is not seen (a server-side check would
//     close it).
//   - A write whose answer was lost is not sent again: Save stays off until
//     the file is read again, and that read says whether it was saved.
//
// Tek bir dosyanın metin düzenleyicisi. Kaydet yalnız metin değiştiyse açılır;
// kapatmak ve sayfadan çıkmak sorar; yazmadan önce dosya yeniden okunur, sunucuda
// değiştiyse hiçbir şey yazılmaz ve kişi seçer; yanıtı yiten yazı yinelenmez.
function FileEditor({ path, opened, post, refusalText, onSaved, onClose }: {
    path: string;
    opened: string;
    post: (path: string, body: Record<string, unknown>) => Promise<Response | null>;
    refusalText: (err: ApiError) => string;
    onSaved: () => void;
    onClose: () => void;
}) {
    const { t } = useI18n();
    const name = path.split('/').pop() || path;
    // The file's text as the server last held it to this editor's knowledge.
    // Bu düzenleyicinin bildiği kadarıyla dosyanın sunucudaki metni.
    const [base, setBase] = useState(opened);
    const [text, setText] = useState(opened);
    // The server's text when it differs from `base`: a change made elsewhere.
    // Sunucudaki metin `base`ten farklıysa: başka yerde yapılmış değişiklik.
    const [server, setServer] = useState<string | null>(null);
    // The text whose write lost its answer; Save stays off until a read decides.
    // Yazısının yanıtı yiten metin; bir okuma karar verene dek Kaydet kapalıdır.
    const [lost, setLost] = useState<string | null>(null);
    const [checking, setChecking] = useState(false);
    const runner = useSaveRun();
    const changed = text !== base;
    useLeaveGuard(changed ? 'some' : 0);

    // The file as the server holds it now; null (said in a toast) when it
    // could not be read. Reading changes nothing.
    // Dosyanın sunucudaki şimdiki hâli; okunamazsa null. Okumak bir şey değiştirmez.
    const readNow = async (): Promise<string | null> => {
        const res = await post(path, { action: 'read' });
        if (res && res.ok) {
            let content: unknown;
            try {
                content = ((await res.json()) as { content?: unknown }).content;
            } catch {
                content = undefined; // not the contract: said below
            }
            if (typeof content === 'string') return content;
        }
        showToast('error', res && !res.ok && !answerWasLost(res) ? refusalText(await readApiError(res)) : t('files.checkFailed'));
        return null;
    };

    const write = (expected: string) => runner.run(async () => {
        // A write whose answer was lost is never sent again unread.
        // Yanıtı yiten yazı okunmadan bir daha gönderilmez.
        if (lost !== null) return 'unknown';
        const sent = text;
        const now = await readNow();
        if (now === null) return 'refused';
        if (now !== expected) {
            setServer(now);
            return 'refused';
        }
        const res = await post(path, { action: 'write', content: sent });
        if (!res || answerWasLost(res)) {
            setLost(sent);
            return 'unknown';
        }
        if (!res.ok) {
            showToast('error', refusalText(await readApiError(res)));
            return 'refused';
        }
        setBase(sent);
        showToast('success', t('files.saved'));
        onSaved();
        return 'saved';
    });
    const save = (expected: string) => void write(expected);

    // After a lost answer: the file read again says what happened.
    // Yanıt yittikten sonra: yeniden okunan dosya ne olduğunu söyler.
    const check = async () => {
        if (lost === null) return;
        setChecking(true);
        const now = await readNow();
        setChecking(false);
        if (now === null) return;
        setLost(null);
        runner.clear();
        if (now === lost) {
            setBase(lost);
            showToast('success', t('files.saved'));
            onSaved();
        } else if (now === base) {
            showToast('info', t('files.notSaved'));
        } else {
            setServer(now);
        }
    };

    const takeServer = async () => {
        if (server === null) return;
        if (changed && !(await confirmAction({
            title: t('files.takeServer'),
            lost: [t('files.yourChanges', { name })],
            confirmLabel: t('files.takeServer'),
            cancelLabel: t('domain.unsaved.back'),
            tone: 'danger',
        }))) return;
        setBase(server);
        setText(server);
        setServer(null);
    };

    const overwrite = async () => {
        if (server === null) return;
        const expected = server;
        // The write runs in the locked progress window; it is checked against
        // the server's text once more first.
        // Yazı kilitli ilerleme penceresinde çalışır; önce sunucudaki metinle
        // bir kez daha karşılaştırılır.
        await confirmAction({
            title: t('files.overwrite'),
            lost: [t('files.overwriteLost', { name })],
            confirmLabel: t('files.overwrite'),
            cancelLabel: t('domain.unsaved.back'),
            tone: 'danger',
            action: async () => {
                setServer(null);
                setBase(expected);
                await write(expected);
            },
        });
    };

    const close = async () => {
        if (changed && !(await confirmAction({
            title: t('domain.unsaved.title'),
            lost: [t('files.yourChanges', { name })],
            kept: [t('files.closeKept')],
            confirmLabel: t('domain.unsaved.close'),
            cancelLabel: t('domain.unsaved.back'),
            tone: 'danger',
        }))) return;
        onClose();
    };

    // Discard: the editor shows the file as the server holds it, the newer
    // text when one was found.
    // Vazgeç: düzenleyici dosyanın sunucudaki hâlini gösterir.
    const discard = () => {
        const shown = server ?? base;
        setServer(null);
        setBase(shown);
        setText(shown);
    };

    const busy = runner.saving || checking;
    return (
        <div className="flex min-h-[540px] flex-col">
            <div className="mb-3 flex items-start justify-between gap-3">
                <div className="min-w-0">
                    <h3 className="break-all text-sm font-semibold text-fg">
                        {name}
                        {changed && <ChangedMark />}
                    </h3>
                    <p className="break-all font-mono text-xs text-fg-subtle">{path}</p>
                </div>
                <Button icon={X} onClick={() => void close()} disabled={busy}>{t('common.close')}</Button>
            </div>
            {server !== null && (
                <div role="alert" data-file-moved="" className="mb-3 flex flex-wrap items-start gap-x-3 gap-y-2 rounded-lg border border-warning/40 bg-warning/10 p-3 text-sm text-fg">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                    <p className="min-w-0 max-w-[75ch] flex-1">{t('files.serverChanged', { name })}</p>
                    <div className="flex flex-wrap gap-2">
                        <Button onClick={() => void takeServer()} disabled={busy}>{t('files.takeServer')}</Button>
                        <Button variant="danger" onClick={() => void overwrite()} disabled={busy || !changed}>{t('files.overwrite')}</Button>
                    </div>
                </div>
            )}
            <textarea
                value={text}
                onChange={(e) => setText(e.target.value)}
                readOnly={busy}
                aria-label={name}
                className="min-h-[24rem] w-full flex-1 resize-y rounded-lg border border-border bg-surface-2 p-4 font-mono text-sm text-fg outline-none focus:border-primary"
                spellCheck={false}
            />
            <SaveBar
                className="-mx-5 px-5"
                count={changed ? 'some' : 0}
                saving={runner.saving}
                blockedBy={server !== null ? t('files.chooseFirst') : undefined}
                unknown={lost !== null ? { held: true, checking, onCheck: () => void check() } : undefined}
                onSave={() => save(base)}
                onDiscard={discard}
            />
        </div>
    );
}

function ToolButton({
    icon: Icon,
    title,
    onClick,
    spin,
    small,
    danger,
    disabled,
}: {
    icon: typeof Folder;
    title: string;
    onClick: () => void;
    spin?: boolean;
    small?: boolean;
    danger?: boolean;
    disabled?: boolean;
}) {
    return (
        <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onClick(); }}
            title={title}
            aria-label={title}
            disabled={disabled}
            className={`rounded-md p-1.5 text-fg-muted transition-colors hover:bg-surface-2 disabled:pointer-events-none disabled:opacity-40 ${danger ? 'hover:text-danger' : 'hover:text-fg'}`}
        >
            <Icon className={`${small ? 'h-3.5 w-3.5' : 'h-4 w-4'} ${spin ? 'animate-spin' : ''}`} />
        </button>
    );
}

function fmtSize(bytes: number): string {
    if (!bytes) return '—';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function fmtDate(dateStr: string, locale: string): string {
    return formatDateTime(new Date(dateStr), locale) || dateStr;
}
