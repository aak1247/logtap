import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Link } from "react-router-dom";

function isExternalHref(href: string) {
  return /^https?:\/\//i.test(href);
}

function toDocRouteFromMarkdownHref(href: string) {
  const clean = href.split("#")[0]?.split("?")[0] ?? "";
  const normalized = clean.replace(/\\/g, "/");
  const base = normalized.split("/").pop() ?? normalized;

  const byBaseName: Record<string, string> = {
    "OVERVIEW.md": "/docs/overview",
    "DEPLOYMENT.md": "/docs/deployment",
    "INGEST.md": "/docs/ingest",
    "SDKs.md": "/docs/sdks",
    "SDK_SPEC.md": "/docs/sdk-spec",
    "javascript.md": "/docs/integrations/javascript",
    "go.md": "/docs/integrations/go",
    "flutter.md": "/docs/integrations/flutter",
    "python.md": "/docs/integrations/python",
    "java.md": "/docs/integrations/java",
    "dotnet.md": "/docs/integrations/dotnet",
    "php.md": "/docs/integrations/php",
    "ruby.md": "/docs/integrations/ruby",
  };

  return byBaseName[base] ?? "";
}

export function Markdown(props: { content: string }) {
  return (
    <div className="text-sm leading-7 text-zinc-200">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          h1: ({ children }) => (
            <h1 className="mb-4 text-2xl font-semibold text-zinc-100">
              {children}
            </h1>
          ),
          h2: ({ children }) => (
            <h2 className="mb-3 mt-8 text-xl font-semibold text-zinc-100">
              {children}
            </h2>
          ),
          h3: ({ children }) => (
            <h3 className="mb-2 mt-6 text-lg font-semibold text-zinc-100">
              {children}
            </h3>
          ),
          p: ({ children }) => <p className="my-3">{children}</p>,
          ul: ({ children }) => (
            <ul className="my-3 list-disc space-y-1 pl-6">{children}</ul>
          ),
          ol: ({ children }) => (
            <ol className="my-3 list-decimal space-y-1 pl-6">{children}</ol>
          ),
          li: ({ children }) => <li className="text-zinc-200">{children}</li>,
          a: ({ href, children }) => {
            const h = href ?? "";
            const mapped = toDocRouteFromMarkdownHref(h);
            if (mapped) {
              return (
                <Link className="text-indigo-300 hover:text-indigo-200" to={mapped}>
                  {children}
                </Link>
              );
            }
            if (!h || !isExternalHref(h)) {
              return (
                <a className="text-indigo-300 hover:text-indigo-200" href={h}>
                  {children}
                </a>
              );
            }
            return (
              <a
                className="text-indigo-300 hover:text-indigo-200"
                href={h}
                target="_blank"
                rel="noreferrer"
              >
                {children}
              </a>
            );
          },
          code: ({ className, children }) => {
            const isBlock = /language-/.test(className ?? "");
            if (isBlock) {
              return <code className={className}>{children}</code>;
            }
            return (
              <code className="rounded bg-zinc-900/60 px-1 py-0.5 text-[12px] text-zinc-100">
                {children}
              </code>
            );
          },
          pre: ({ children }) => (
            <pre className="my-4 overflow-auto rounded-lg border border-zinc-900 bg-zinc-950 p-4 text-xs text-zinc-100">
              {children}
            </pre>
          ),
          blockquote: ({ children }) => (
            <blockquote className="my-4 rounded-md border border-zinc-900 bg-zinc-950/40 px-4 py-3 text-zinc-300">
              {children}
            </blockquote>
          ),
          hr: () => <hr className="my-6 border-zinc-900" />,
          table: ({ children }) => (
            <div className="my-4 overflow-auto">
              <table className="w-full border-collapse text-left text-xs">
                {children}
              </table>
            </div>
          ),
          thead: ({ children }) => (
            <thead className="border-b border-zinc-900 text-zinc-300">
              {children}
            </thead>
          ),
          tbody: ({ children }) => (
            <tbody className="divide-y divide-zinc-900">{children}</tbody>
          ),
          tr: ({ children }) => <tr className="align-top">{children}</tr>,
          th: ({ children }) => (
            <th className="whitespace-nowrap px-3 py-2 font-semibold">
              {children}
            </th>
          ),
          td: ({ children }) => <td className="px-3 py-2">{children}</td>,
        }}
      >
        {props.content}
      </ReactMarkdown>
    </div>
  );
}

