import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

/**
 * Renders the agent's prose as markdown. The model replies in markdown
 * (**bold**, ### headings, > quotes, lists, ---), so showing it verbatim
 * would leak the syntax. react-markdown does not render raw HTML by
 * default, so model output cannot inject markup. Links open in the
 * default browser rather than navigating the app away.
 *
 * Same shape as the 1.0 companion's `components/companion/Markdown.tsx`,
 * which was deleted with the rest of that panel but left `react-markdown`
 * and `remark-gfm` in package.json for this issue (#95).
 */
export function Markdown({ text }: { text: string }) {
  return (
    <div className="agent-md">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ node: _node, ...props }) => <a {...props} target="_blank" rel="noreferrer" />,
        }}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
}
