/**
 * Token request: email in, review-and-respond workflow behind it.
 *
 * Bot defence is a honeypot field ("website") rendered off-screen but
 * present in the DOM — humans never see or fill it, naive bots do. It is
 * `aria-hidden` and untabbable so assistive technology never lands on it.
 */

import { useId, useState } from 'react';

import { ApiError, requestToken } from '../lib/api.js';

/** Outcome banner state. */
interface Banner {
  readonly tone: 'success' | 'error';
  readonly message: string;
}

/** Renders the token-request section. */
export function TokenRequestCard(): JSX.Element {
  const [email, setEmail] = useState('');
  const [honeypot, setHoneypot] = useState('');
  const [busy, setBusy] = useState(false);
  const [banner, setBanner] = useState<Banner | null>(null);
  const emailId = useId();
  const honeypotId = useId();

  const submit = (): void => {
    setBusy(true);
    setBanner(null);
    requestToken(email.trim(), honeypot)
      .then((response) => {
        setBanner({
          tone: response.outcome === 'accepted' ? 'success' : 'error',
          message: response.message,
        });
        if (response.outcome === 'accepted') {
          setEmail('');
        }
      })
      .catch((cause: unknown) => {
        setBanner({
          tone: 'error',
          message:
            cause instanceof ApiError ? cause.message : 'Something went wrong. Please try again.',
        });
      })
      .finally(() => {
        setBusy(false);
      });
  };

  return (
    <section
      aria-label="Request an API token"
      className="mx-auto w-full max-w-xl rounded-2xl border border-arcane bg-obsidian/60 p-6 sm:p-8"
    >
      <h2 className="font-display text-xl tracking-[0.15em] text-parchment uppercase">
        Request a token
      </h2>
      <p className="mt-2 text-sm leading-relaxed text-mist">
        Every API call requires an authorization token. Leave your email and a team member will
        review your request.
      </p>

      <form
        className="mt-5 flex flex-col gap-4 sm:flex-row sm:items-end"
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <div className="flex flex-1 flex-col gap-1.5">
          <label htmlFor={emailId} className="text-[0.65rem] tracking-[0.2em] text-mist uppercase">
            Email
          </label>
          <input
            id={emailId}
            type="email"
            required
            value={email}
            onChange={(event) => {
              setEmail(event.target.value);
            }}
            placeholder="you@example.com"
            className="w-full rounded-lg border border-arcane bg-void/70 px-3 py-2 text-sm text-parchment placeholder:text-mist/40"
          />
        </div>

        {/* Honeypot: visually removed, untabbable, hidden from AT. */}
        <div aria-hidden="true" className="absolute -left-[9999px] top-auto h-px w-px overflow-hidden">
          <label htmlFor={honeypotId}>Website</label>
          <input
            id={honeypotId}
            type="text"
            tabIndex={-1}
            autoComplete="off"
            value={honeypot}
            onChange={(event) => {
              setHoneypot(event.target.value);
            }}
          />
        </div>

        <button
          type="submit"
          disabled={busy || email.trim() === ''}
          aria-busy={busy}
          className="rounded-full border border-amethyst/40 bg-amethyst/10 px-6 py-2 text-xs tracking-[0.25em] text-lilac uppercase transition-all duration-300 hover:enabled:border-amethyst/70 hover:enabled:bg-amethyst/20 hover:enabled:text-parchment disabled:cursor-not-allowed disabled:opacity-50"
        >
          {busy ? 'Sending…' : 'Send'}
        </button>
      </form>

      {banner !== null && (
        <p
          role={banner.tone === 'error' ? 'alert' : 'status'}
          className={`mt-4 text-sm leading-relaxed ${
            banner.tone === 'success' ? 'text-method-get' : 'text-method-delete'
          }`}
        >
          {banner.message}
        </p>
      )}
    </section>
  );
}
