import rehypeRaw from 'rehype-raw';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

const markdownAllowedElements = [
    'a',
    'blockquote',
    'br',
    'code',
    'del',
    'details',
    'em',
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'hr',
    'img',
    'li',
    'ol',
    'p',
    'pre',
    'strong',
    'summary',
    'table',
    'tbody',
    'td',
    'th',
    'thead',
    'tr',
    'ul',
];

export default function MarkdownContent({children}: {children: string}) {
    return (
        <ReactMarkdown
            allowedElements={markdownAllowedElements}
            rehypePlugins={[rehypeRaw]}
            remarkPlugins={[remarkGfm]}
            components={{
                a({href, children}) {
                    const safeHref = safeMarkdownLink(href);
                    if (!safeHref) {
                        return <>{children}</>;
                    }
                    return <a href={safeHref} target="_blank" rel="noreferrer">{children}</a>;
                },
                img({src, alt}) {
                    const safeSrc = safeMarkdownImage(src);
                    if (!safeSrc) {
                        return null;
                    }
                    return <img src={safeSrc} alt={alt ?? ''} loading="lazy"/>;
                },
                details({children}) {
                    return <details className="markdown-details">{children}</details>;
                },
                summary({children}) {
                    return <summary>{children}</summary>;
                }
            }}
        >
            {children}
        </ReactMarkdown>
    );
}

function safeMarkdownLink(value?: string) {
    const url = safeMarkdownURL(value, ['http:', 'https:', 'mailto:']);
    if (url) {
        return url;
    }
    const trimmed = value?.trim() ?? '';
    return trimmed.startsWith('#') ? trimmed : '';
}

function safeMarkdownImage(value?: string) {
    return safeMarkdownURL(value, ['http:', 'https:']);
}

function safeMarkdownURL(value: string | undefined, allowedProtocols: string[]) {
    const trimmed = value?.trim() ?? '';
    if (!trimmed) {
        return '';
    }
    try {
        const url = new URL(trimmed);
        return allowedProtocols.includes(url.protocol) ? url.toString() : '';
    } catch {
        return '';
    }
}
