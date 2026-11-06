import type { ReactNode } from 'react';
import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { useNavigate } from '../router';

// The administrator's one next step beside a sentence that names what is
// missing or closed (alpha.127; owner on Boston, 6 Nov 2026: the viewer IS the
// administrator, so "the administrator installs it from Components" with no
// action is wrong).
//
// - `component`: the button opens that component's own install window on the
//   Components page, where what comes with it is reviewed and nothing starts
//   before it is confirmed.
// - `to` and `label`: the button opens the exact page where the administrator
//   decides (Catalog management, PHP-FPM, the mail setup).
// - `admin`: the administrator's own sentence, said instead of `fallback`.
//
// Anyone else sees `fallback` (nothing by default): their sentence says who
// acts. Nothing here starts a change by itself.
// Eksik ya da kapalı olanı adlandıran cümlenin yanında yöneticinin tek
// sonraki adımı; başkası yalnız kimin yapacağını söyleyen cümleyi görür.
export function AdminInstallLink({ component, to, label, admin, fallback = null }: {
    component?: string;
    to?: string;
    label?: string;
    admin?: ReactNode;
    fallback?: ReactNode;
}) {
    const { t } = useI18n();
    const { role } = useAuth();
    const navigate = useNavigate();
    if (role !== 'admin') return <>{fallback}</>;
    const target = to ?? (component ? `/components?component=${encodeURIComponent(component)}&install=` : '');
    const text = label ?? (component ? t('services.installTitle', { name: t(`services.component.name.${component}` as TranslationKey) }) : '');
    return (
        <>
            {admin}
            {admin && target ? ' ' : null}
            {target && (
                <button
                    type="button"
                    onClick={() => navigate(target)}
                    className="font-semibold text-primary underline underline-offset-2 max-sm:min-h-11"
                    data-install-link={component}
                    data-admin-step={to}
                >
                    {text}
                </button>
            )}
        </>
    );
}
