import { useState, type InputHTMLAttributes } from 'react';
import { CheckCircle2, XCircle } from 'lucide-react';
import { inputClass } from '../../ui';
import type { ConfirmAnswer, ConfirmRequest, TypedName } from '../../../lib/confirmAction';
import { useSaveWords } from './words';

// The structured part of the product's confirmation (alpha.132, drawing X16):
// what is lost (red cross) and what stays (green check), one plain line each;
// one heavier choice ("also delete the data"); and the exact name the person
// types before a destructive button turns on. ../../ConfirmHost.tsx fetches
// this part only for a question that has one of them, so the boot path does
// not carry it. The typed name is also the one field the dialogues that ask
// more than a question use (SecurityEmergency.tsx); whether it matches is
// decided once, by typedNameMatches here.
//
// Ürünün onay penceresinin yapılı kısmı: ne kaybolur, ne değişmez, daha ağır
// tek seçim ve yıkıcı düğmeden önce yazılan tam ad. Yazılan adın eşleşmesine
// tek yerde, burada karar verilir.

/** Whether what was typed is the name. Spaces around it do not count. */
export function typedNameMatches(typed: string, name: TypedName): boolean {
    const given = typed.trim();
    return name.ignoreCase ? given.toLowerCase() === name.value.toLowerCase() : given === name.value;
}

/** What is lost and what stays, one line each. Nothing when both are empty. */
export function ConfirmLists({ lost = [], kept = [] }: { lost?: string[]; kept?: string[] }) {
    const say = useSaveWords();
    if (lost.length === 0 && kept.length === 0) return null;
    const row = 'flex items-start gap-2 px-3 py-2';
    return (
        <ul className="divide-y divide-border rounded-lg border border-border text-sm text-fg" data-confirm-lists="">
            {lost.map((line, index) => (
                <li key={`lost-${index}`} className={row} data-confirm-lost="">
                    <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-danger" aria-hidden="true" />
                    <span><span className="sr-only">{say('lostMark')} </span>{line}</span>
                </li>
            ))}
            {kept.map((line, index) => (
                <li key={`kept-${index}`} className={row} data-confirm-kept="">
                    <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" aria-hidden="true" />
                    <span><span className="sr-only">{say('keptMark')} </span>{line}</span>
                </li>
            ))}
        </ul>
    );
}

/** The field the exact name is typed into. Its label names what to type. */
export function TypedNameField({ id, label, value, onChange, ...input }: {
    id: string;
    label: string;
    value: string;
    onChange: (value: string) => void;
} & Omit<InputHTMLAttributes<HTMLInputElement>, 'id' | 'value' | 'onChange'>) {
    return (
        <div>
            <label htmlFor={id} className="mb-1 block text-sm font-medium text-fg-muted">{label}</label>
            <input
                {...input}
                id={id}
                className={`${inputClass} field-lg font-mono`}
                autoComplete="off"
                spellCheck={false}
                value={value}
                onChange={(event) => onChange(event.target.value)}
            />
        </div>
    );
}

/**
 * The body of a structured question, drawn by ConfirmHost. It keeps the
 * choice and the typed name, and tells the host both, with whether the name
 * matches (the confirm button waits for it).
 */
export default function ConfirmDetails({ request, disabled, onAnswer }: {
    request: ConfirmRequest;
    disabled: boolean;
    onAnswer: (answer: ConfirmAnswer, matches: boolean) => void;
}) {
    const [option, setOption] = useState(false);
    const [typed, setTyped] = useState('');
    const choice = request.option;
    const nameFor = (chosen: boolean) => request.typedName ?? (chosen ? choice?.typedName : undefined);
    const typedName = nameFor(option);
    const tell = (chosen: boolean, text: string) => {
        const name = nameFor(chosen);
        onAnswer({ option: chosen, typed: text }, !name || typedNameMatches(text, name));
    };
    const onOption = (chosen: boolean) => { setOption(chosen); tell(chosen, typed); };
    const onTyped = (text: string) => { setTyped(text); tell(option, text); };
    const lost = [...(request.lost ?? []), ...(option ? choice?.lost ?? [] : [])];
    return (
        <div className="space-y-4">
            <ConfirmLists lost={lost} kept={request.kept} />
            {choice && (
                <label className="flex items-start gap-2 text-sm text-fg">
                    <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary" checked={option} disabled={disabled}
                        onChange={(event) => onOption(event.target.checked)} data-confirm-option="" />
                    {choice.label}
                </label>
            )}
            {typedName && (
                <TypedNameField id="confirm-action-typed" label={typedName.label} value={typed} onChange={onTyped} disabled={disabled} data-confirm-typed="" />
            )}
        </div>
    );
}
