import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { useNavigate } from '../router';
import { setupProblemGuide, setupProblemsID, type SetupProblemFix, type SetupStartBlock } from '../lib/setupProblems';
import { BlockedReason, BlockingProblems, type BlockingProblem } from './BlockingProblems';

// The setup Summary's blockers in the shared problem pattern, at the top of
// the Summary (owner feedback 2026-10-31). lib/setupProblems.ts maps each code
// to its words and fix; a code without words keeps the sentence the screen
// gave it before (fallback). onFix performs the wizard's own fixes (a step,
// removing Email, reading the plan again); My account is opened here.
// Kurulum Ozeti'nin engelleri ortak sorun biciminde, Ozet'in en ustunde.

export function SetupPlanProblems({ blockers, fallback, name, busy, onFix }: {
    blockers: string[];
    /** The sentence the screen already had for a code (ServerSetup's failureText). */
    fallback: (code: string) => string;
    /** A component id in the words the screen uses for it. */
    name: (id: string) => string;
    busy: boolean;
    onFix: (fix: SetupProblemFix) => void;
}) {
    const { t } = useI18n();
    const navigate = useNavigate();
    const label = (fix: SetupProblemFix) => fix.kind === 'step' ? t('setup.problem.fix.step', { step: t(`setup.step.${fix.step}` as TranslationKey) })
        : fix.kind === 'removeMail' ? t('setup.problem.fix.removeMail')
            : fix.kind === 'recheck' ? t('setup.problem.fix.recheck')
                : t('setup.blocker.contactAction');
    const problems: BlockingProblem[] = blockers.map(code => {
        const guide = setupProblemGuide(code);
        if (!guide) return { code, what: fallback(code) };
        const values = { ...guide.values };
        for (const key of guide.components || []) values[key] = name(values[key]);
        const fix = guide.fix;
        return {
            code,
            what: t(`${guide.key}.what` as TranslationKey, values),
            todo: t(`${guide.key}.todo` as TranslationKey, values),
            ...(fix ? { fix: {
                label: label(fix),
                busy: busy && fix.kind === 'recheck',
                disabled: busy,
                onFix: () => fix.kind === 'account' ? navigate('/account#email') : onFix(fix),
            } } : {}),
        };
    });
    return <BlockingProblems id={setupProblemsID} className="mt-5" problems={problems}
        title={t(problems.length === 1 ? 'setup.problem.title.one' : 'setup.problem.title.many', { count: problems.length })} />;
}

/** The reason beside the disabled Start button. */
export function SetupStartReason({ id, block }: { id: string; block: SetupStartBlock }) {
    const { t } = useI18n();
    const text = block.reason === 'problems' ? t(block.count === 1 ? 'setup.problem.blocked.one' : 'setup.problem.blocked.many', { count: block.count })
        : t(`setup.problem.blocked.${block.reason}` as TranslationKey);
    return <BlockedReason id={id} target={block.target} text={text} />;
}
