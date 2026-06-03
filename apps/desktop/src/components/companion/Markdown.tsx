import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

/**
 * Renders companion prose as markdown. The model replies in markdown
 * (**bold**, ### headings, > quotes, lists, ---), so showing it verbatim
 * leaks the syntax. react-markdown does not render raw HTML by default, so
 * model output cannot inject markup. Links open in the default browser.
 */
export function Markdown({ text }: { text: string }) {
  return (
    <div className="cmp-md">
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
