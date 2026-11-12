// The router's one question before it leaves a page with unsaved changes
// (alpha.132). The router is on the boot path and the drafts are not, so the
// two meet here: a module without imports that the router asks and that
// ./leaveGuard.ts answers once a screen with a draft has loaded. With nothing
// registered, or nothing unsaved, the answer is null and the router goes on.
//
// Kaydedilmemiş değişiklik varken yönlendiricinin sorduğu tek soru. Yönlendirici
// açılış yolundadır, taslaklar değildir; ikisi bu içe aktarmasız modülde
// buluşur. Kayıtlı soran yoksa ya da kaydedilmemiş bir şey yoksa yanıt null'dır.

/** null: nothing unsaved, go on. A promise: the question was asked; true means leave. */
export type LeaveAsker = () => Promise<boolean> | null;

let asker: LeaveAsker | null = null;

export function setLeaveAsker(next: LeaveAsker | null): void {
    asker = next;
}

export function askToLeave(): Promise<boolean> | null {
    return asker ? asker() : null;
}
