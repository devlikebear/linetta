import ReactMarkdown from "react-markdown";
import remarkCjkFriendly from "remark-cjk-friendly";
import remarkGfm from "remark-gfm";
import type { AgentCitation } from "../../lib/types";

/**
 * Renders the agent's prose as markdown. The model replies in markdown
 * (**bold**, ### headings, > quotes, lists, ---), so showing it verbatim
 * would leak the syntax. react-markdown does not render raw HTML by
 * default, so model output cannot inject markup. Links open in the
 * default browser rather than navigating the app away.
 *
 * remark-cjk-friendly relaxes CommonMark's flanking rule, which otherwise
 * leaves the ** literal in `**설화(할아버지)**이고`: Korean and Japanese
 * attach particles straight after the closing delimiter.
 *
 * Same shape as the 1.0 companion's `components/companion/Markdown.tsx`,
 * which was deleted with the rest of that panel but left `react-markdown`
 * and `remark-gfm` in package.json for this issue (#95).
 *
 * With `citations`, a reply's [1], [2] become chips for the sources its turn
 * checked (#385). Only a number that names one of them is turned: the same
 * brackets in a reply with no checked sources, or a number past them, stay
 * text, so a chip is never drawn for a source nobody verified.
 */
export function Markdown({
  text,
  citations,
  onCite,
  citeTitle,
}: {
  text: string;
  citations?: AgentCitation[];
  onCite?: (citation: AgentCitation) => void;
  /** The chip's hover text: where the source is and what it says. */
  citeTitle?: (citation: AgentCitation) => string;
}) {
  const byNumber = new Map((citations ?? []).map((c) => [c.n, c]));
  return (
    <div className="agent-md">
      <ReactMarkdown
        remarkPlugins={byNumber.size > 0 ? [remarkGfm, remarkCjkFriendly, citeRefs(new Set(byNumber.keys()))] : [remarkGfm, remarkCjkFriendly]}
        components={{
          a: ({ node: _node, ...props }) => <a {...props} target="_blank" rel="noreferrer" />,
          cite: ({ node: _node, ...props }) => {
            const n = Number((props as Record<string, unknown>)["data-cite"]);
            const citation = byNumber.get(n);
            if (!citation) return <cite {...props} />;
            return (
              <button
                type="button"
                className="agent-cite"
                data-testid={`agent-cite-${n}`}
                title={citeTitle?.(citation)}
                disabled={!onCite}
                onClick={() => onCite?.(citation)}
              >
                {n}
              </button>
            );
          },
        }}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
}

interface MdNode {
  type: string;
  value?: string;
  children?: MdNode[];
  data?: Record<string, unknown>;
}

const CITE = /\[(\d{1,2})\]/g;

/** A remark plugin turning [n] in plain text into a `cite` element carrying n,
 *  for each n in `numbers`. Code and link text are left alone: only `text`
 *  nodes are split, and a link's label is a child of the link, kept whole. */
function citeRefs(numbers: Set<number>) {
  return () => (tree: MdNode) => {
    const walk = (node: MdNode) => {
      if (!node.children || node.type === "link" || node.type === "linkReference") return;
      const out: MdNode[] = [];
      for (const child of node.children) {
        if (child.type !== "text" || !child.value) {
          walk(child);
          out.push(child);
          continue;
        }
        let last = 0;
        for (const m of child.value.matchAll(CITE)) {
          const n = Number(m[1]);
          if (!numbers.has(n)) continue;
          if (m.index > last) out.push({ type: "text", value: child.value.slice(last, m.index) });
          out.push({
            type: "citeRef",
            children: [{ type: "text", value: String(n) }],
            data: { hName: "cite", hProperties: { dataCite: String(n) } },
          });
          last = m.index + m[0].length;
        }
        if (last === 0) {
          out.push(child);
        } else if (last < child.value.length) {
          out.push({ type: "text", value: child.value.slice(last) });
        }
      }
      node.children = out;
    };
    walk(tree);
  };
}
