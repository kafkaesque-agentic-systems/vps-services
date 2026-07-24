/**
 * Shared building blocks of the API explorer: method badge, labelled field,
 * copyable curl snippet, JSON result view, and the operation card shell.
 */

import { useId, useState, type ReactNode } from 'react';

/* ------------------------------------------------------------------------- */
/* Method badge                                                              */
/* ------------------------------------------------------------------------- */

/** HTTP methods the explorer demonstrates. */
export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE';

/** Tokenised badge colour per method (see tailwind.config). */
const METHOD_CLASSES: Record<HttpMethod, string> = {
  GET: 'text-method-get border-method-get/40',
  POST: 'text-method-post border-method-post/40',
  PUT: 'text-method-put border-method-put/40',
  DELETE: 'text-method-delete border-method-delete/40',
};

/** Renders a small method chip. */
export function MethodBadge({ method }: { readonly method: HttpMethod }): JSX.Element {
  return (
    <span
      className={`inline-flex min-w-[3.5rem] items-center justify-center rounded-md border bg-void/60 px-2 py-0.5 font-mono text-[0.7rem] font-semibold tracking-wider ${METHOD_CLASSES[method]}`}
    >
      {method}
    </span>
  );
}

/* ------------------------------------------------------------------------- */
/* Labelled field                                                            */
/* ------------------------------------------------------------------------- */

/** Shared styling for text inputs across the explorer. */
export const FIELD_CLASSES =
  'w-full rounded-lg border border-arcane bg-void/70 px-3 py-2 text-sm text-parchment placeholder:text-mist/40';

/** A labelled single-line input. */
export function Field({
  label,
  value,
  onChange,
  placeholder,
  type = 'text',
}: {
  readonly label: string;
  readonly value: string;
  readonly onChange: (next: string) => void;
  readonly placeholder?: string;
  readonly type?: 'text' | 'password';
}): JSX.Element {
  const id = useId();
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-[0.65rem] tracking-[0.2em] text-mist uppercase">
        {label}
      </label>
      <input
        id={id}
        type={type}
        value={value}
        {...(placeholder === undefined ? {} : { placeholder })}
        onChange={(event) => {
          onChange(event.target.value);
        }}
        className={FIELD_CLASSES}
      />
    </div>
  );
}

/* ------------------------------------------------------------------------- */
/* Curl snippet                                                              */
/* ------------------------------------------------------------------------- */

/** A copyable curl example inside a code well. */
export function CurlSnippet({ command }: { readonly command: string }): JSX.Element {
  const [copied, setCopied] = useState(false);

  return (
    <div className="relative">
      <pre className="overflow-x-auto rounded-lg border border-arcane bg-void/80 p-3 pr-16 font-mono text-[0.7rem] leading-relaxed text-mist">
        {command}
      </pre>
      <button
        type="button"
        onClick={() => {
          void navigator.clipboard.writeText(command).then(() => {
            setCopied(true);
            setTimeout(() => {
              setCopied(false);
            }, 1500);
          });
        }}
        className="absolute top-2 right-2 rounded-md border border-arcane bg-obsidian px-2 py-1 text-[0.6rem] tracking-widest text-mist uppercase transition-colors hover:border-amethyst/50 hover:text-lilac"
      >
        {copied ? 'Copied' : 'Copy'}
      </button>
    </div>
  );
}

/* ------------------------------------------------------------------------- */
/* JSON result view                                                          */
/* ------------------------------------------------------------------------- */

/** One highlighted token of pretty-printed JSON. */
interface JsonToken {
  readonly text: string;
  readonly kind: 'key' | 'string' | 'number' | 'literal' | 'plain';
}

/** Tokenises pretty-printed JSON for highlighting. Small and total: anything
 * unmatched falls through as plain text, so it can never fail to render. */
function tokeniseJson(pretty: string): readonly JsonToken[] {
  const pattern =
    /("(?:\\.|[^"\\])*")(\s*:)?|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|(\btrue\b|\bfalse\b|\bnull\b)/g;
  const tokens: JsonToken[] = [];
  let last = 0;
  for (let match = pattern.exec(pretty); match !== null; match = pattern.exec(pretty)) {
    if (match.index > last) {
      tokens.push({ text: pretty.slice(last, match.index), kind: 'plain' });
    }
    if (match[1] !== undefined) {
      tokens.push({ text: match[1], kind: match[2] === undefined ? 'string' : 'key' });
      if (match[2] !== undefined) {
        tokens.push({ text: match[2], kind: 'plain' });
      }
    } else if (match[3] !== undefined) {
      tokens.push({ text: match[3], kind: 'number' });
    } else if (match[4] !== undefined) {
      tokens.push({ text: match[4], kind: 'literal' });
    }
    last = pattern.lastIndex;
  }
  if (last < pretty.length) {
    tokens.push({ text: pretty.slice(last), kind: 'plain' });
  }
  return tokens;
}

/** Colour classes per token kind. */
const TOKEN_CLASSES: Record<JsonToken['kind'], string> = {
  key: 'text-lilac',
  string: 'text-parchment/90',
  number: 'text-gilt',
  literal: 'text-method-get',
  plain: 'text-mist',
};

/** Pretty-printed, syntax-highlighted JSON in a scrollable well. */
export function JsonView({ data }: { readonly data: unknown }): JSX.Element {
  const pretty = JSON.stringify(data, null, 2) ?? 'null';
  return (
    <pre className="max-h-72 overflow-auto rounded-lg border border-arcane bg-void/80 p-3 font-mono text-[0.7rem] leading-relaxed">
      {tokeniseJson(pretty).map((token, index) => (
        // Index keys are safe: the token list is derived, never reordered.
        // eslint-disable-next-line react/no-array-index-key
        <span key={index} className={TOKEN_CLASSES[token.kind]}>
          {token.text}
        </span>
      ))}
    </pre>
  );
}

/* ------------------------------------------------------------------------- */
/* Operation card shell                                                      */
/* ------------------------------------------------------------------------- */

/** Props for {@link OperationCard}. */
export interface OperationCardProps {
  /** Anchor id for the operation index. */
  readonly id: string;
  readonly method: HttpMethod;
  /** Public endpoint path shown beside the badge, e.g. `/quote/search`. */
  readonly path: string;
  readonly title: string;
  readonly description: string;
  /** Public curl example for API consumers. */
  readonly curl: string;
  /** True when the operation needs the caller's own API token. */
  readonly authorized?: boolean;
  /** The operation's form and result area. */
  readonly children: ReactNode;
}

/** Renders one operation's card. */
export function OperationCard({
  id,
  method,
  path,
  title,
  description,
  curl,
  authorized = false,
  children,
}: OperationCardProps): JSX.Element {
  return (
    <article
      id={id}
      aria-label={title}
      className="scroll-mt-24 rounded-2xl border border-arcane bg-obsidian/60 p-5 sm:p-6"
    >
      <header className="flex flex-wrap items-center gap-3">
        <MethodBadge method={method} />
        <code className="font-mono text-sm text-parchment/90">{path}</code>
        {authorized && (
          <span className="rounded-md border border-gilt/30 px-2 py-0.5 text-[0.6rem] tracking-widest text-gilt uppercase">
            Token required
          </span>
        )}
      </header>

      <h3 className="mt-3 font-display text-lg tracking-wide text-parchment">{title}</h3>
      <p className="mt-1 text-sm leading-relaxed text-mist">{description}</p>

      <div className="mt-4">
        <CurlSnippet command={curl} />
      </div>

      <div className="mt-4 flex flex-col gap-4">{children}</div>
    </article>
  );
}

/* ------------------------------------------------------------------------- */
/* Run button + result state                                                 */
/* ------------------------------------------------------------------------- */

/** Small primary action button used by every panel. */
export function RunButton({
  label,
  busy,
  disabled = false,
}: {
  readonly label: string;
  readonly busy: boolean;
  readonly disabled?: boolean;
}): JSX.Element {
  return (
    <button
      type="submit"
      disabled={busy || disabled}
      aria-busy={busy}
      className="self-start rounded-full border border-amethyst/40 bg-amethyst/10 px-6 py-2 text-xs tracking-[0.25em] text-lilac uppercase transition-all duration-300 hover:enabled:border-amethyst/70 hover:enabled:bg-amethyst/20 hover:enabled:text-parchment disabled:cursor-not-allowed disabled:opacity-50"
    >
      {busy ? 'Running…' : label}
    </button>
  );
}
