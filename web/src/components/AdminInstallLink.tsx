import type { ReactNode } from 'react';
import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { useNavigate } from '../router';

// The administrator's one next step beside a sentence that names a missing
// component (alpha.127; owner on Boston, 6 Nov 2026: the viewer IS the
// administrator, so "the administrator installs it from Components" with no
// action is wrong). It opens that component's own install window on the
// Components page, where what comes with it is reviewed and nothing starts
// before it is confirmed. Anyone else sees `fallback` (nothing by default):
// their sentence says who acts.
// Eksik bir bileşeni adlandıran cümlenin yanında yöneticinin tek sonraki
// adımı: bileşenin kendi kurulum penceresi. Başkası burada bir şey görmez.
export function AdminInstallLink({ component, fallback = null }: { component: string; fallback?: ReactNode }) {
    const { t } = useI18n();
    const { role } = useAuth();
    const navigate = useNavigate();
    if (role !== 'admin') return <>{fallback}</>;
    return (
        <button
            type="button"
            onClick={() => navigate(`/components?component=${encodeURIComponent(component)}&install=`)}
            className="font-semibold text-primary underline underline-offset-2 max-sm:min-h-11"
            data-install-link={component}
        >
            {t('services.installTitle', { name: t(`services.component.name.${component}` as TranslationKey) })}
        </button>
    );
}
