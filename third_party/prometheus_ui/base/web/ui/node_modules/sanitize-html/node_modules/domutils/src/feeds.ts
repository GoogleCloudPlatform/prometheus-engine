import type { AnyNode, Element } from "domhandler";
import { getElementsByTagName } from "./legacy.js";
import { textContent } from "./stringify.js";

/**
 * The medium of a media item.
 *
 * @category Feeds
 */
export type FeedItemMediaMedium =
    | "image"
    | "audio"
    | "video"
    | "document"
    | "executable";

/**
 * The type of a media item.
 *
 * @category Feeds
 */
export type FeedItemMediaExpression = "sample" | "full" | "nonstop";

/**
 * A media item of a feed entry.
 *
 * @category Feeds
 */
export interface FeedItemMedia {
    medium: FeedItemMediaMedium | undefined;
    isDefault: boolean;
    url?: string;
    fileSize?: number;
    type?: string;
    expression?: FeedItemMediaExpression;
    bitrate?: number;
    framerate?: number;
    samplingrate?: number;
    channels?: number;
    duration?: number;
    height?: number;
    width?: number;
    lang?: string;
}

/**
 * An entry of a feed.
 *
 * @category Feeds
 */
export interface FeedItem {
    id?: string;
    title?: string;
    link?: string;
    description?: string;
    pubDate?: Date;
    media: FeedItemMedia[];
}

/**
 * The root of a feed.
 *
 * @category Feeds
 */
export interface Feed {
    type: string;
    id?: string;
    title?: string;
    link?: string;
    description?: string;
    updated?: Date;
    author?: string;
    items: FeedItem[];
}

/**
 * Get the feed object from the root of a DOM tree.
 *
 * @category Feeds
 * @param document The DOM to extract the feed from.
 * @returns The feed.
 */
export function getFeed(document: AnyNode[]): Feed | null {
    const feedRoot = getOneElement(isValidFeed, document);

    return feedRoot
        ? feedRoot.name === "feed"
            ? getAtomFeed(feedRoot)
            : getRssFeed(feedRoot)
        : null;
}

/**
 * Parse an Atom feed.
 *
 * @param feedRoot The root of the feed.
 * @returns The parsed feed.
 */
function getAtomFeed(feedRoot: Element) {
    const childs = feedRoot.children;

    const feed: Feed = {
        type: "atom",
        items: getElementsByTagName("entry", childs).map((item) => {
            const { children } = item;
            const entry: FeedItem = { media: getMediaElements(children) };

            addConditionally(entry, "id", "id", children);
            addConditionally(entry, "title", "title", children);

            const href = getOneElement("link", children)?.attribs["href"];
            if (href) {
                entry.link = href;
            }

            const description =
                fetch("summary", children) || fetch("content", children);
            if (description) {
                entry.description = description;
            }

            const pubDate = fetch("updated", children);
            if (pubDate) {
                entry.pubDate = new Date(pubDate);
            }

            return entry;
        }),
    };

    addConditionally(feed, "id", "id", childs);
    addConditionally(feed, "title", "title", childs);
    const href = getOneElement("link", childs)?.attribs["href"];
    if (href) {
        feed.link = href;
    }
    addConditionally(feed, "description", "subtitle", childs);

    const updated = fetch("updated", childs);
    if (updated) {
        feed.updated = new Date(updated);
    }

    addConditionally(feed, "author", "email", childs, true);

    return feed;
}

/**
 * Parse a RSS feed.
 *
 * @param feedRoot The root of the feed.
 * @returns The parsed feed.
 */
function getRssFeed(feedRoot: Element) {
    const childs = getOneElement("channel", feedRoot.children)?.children ?? [];

    const feed: Feed = {
        type: feedRoot.name.substr(0, 3),
        id: "",
        items: getElementsByTagName("item", feedRoot.children).map(
            (item: Element) => {
                const { children } = item;
                const entry: FeedItem = { media: getMediaElements(children) };
                addConditionally(entry, "id", "guid", children);
                addConditionally(entry, "title", "title", children);
                addConditionally(entry, "link", "link", children);
                addConditionally(entry, "description", "description", children);
                const pubDate =
                    fetch("pubDate", children) || fetch("dc:date", children);
                if (pubDate) entry.pubDate = new Date(pubDate);

                return entry;
            },
        ),
    };

    addConditionally(feed, "title", "title", childs);
    addConditionally(feed, "link", "link", childs);
    addConditionally(feed, "description", "description", childs);

    const updated = fetch("lastBuildDate", childs);
    if (updated) {
        feed.updated = new Date(updated);
    }

    addConditionally(feed, "author", "managingEditor", childs, true);

    return feed;
}

const MEDIA_KEYS_STRING = ["url", "type", "lang"] as const;
const MEDIA_KEYS_INT = [
    "fileSize",
    "bitrate",
    "framerate",
    "samplingrate",
    "channels",
    "duration",
    "height",
    "width",
] as const;

/**
 * Get all media elements of a feed item.
 *
 * @param where Nodes to search in.
 * @returns Media elements.
 */
function getMediaElements(where: AnyNode[]): FeedItemMedia[] {
    return getElementsByTagName("media:content", where).map((element) => {
        const { attribs } = element;

        const media: FeedItemMedia = {
            medium: attribs["medium"] as unknown as
                | FeedItemMediaMedium
                | undefined,
            isDefault: !!attribs["isDefault"],
        };

        for (const attrib of MEDIA_KEYS_STRING) {
            if (attribs[attrib]) {
                media[attrib] = attribs[attrib];
            }
        }

        for (const attrib of MEDIA_KEYS_INT) {
            if (attribs[attrib]) {
                media[attrib] = Number.parseInt(attribs[attrib], 10);
            }
        }

        if (attribs["expression"]) {
            media.expression = attribs[
                "expression"
            ] as unknown as FeedItemMediaExpression;
        }

        return media;
    });
}

/**
 * Get one element by tag name.
 *
 * @param tagName Tag name to look for
 * @param node Node to search in
 * @returns The element or null
 */
function getOneElement(
    tagName: string | ((name: string) => boolean),
    node: AnyNode[],
): Element | null {
    return getElementsByTagName(tagName, node, true, 1)[0];
}

/**
 * Get the text content of an element with a certain tag name.
 *
 * @param tagName Tag name to look for.
 * @param where Node to search in.
 * @param recurse Whether to recurse into child nodes.
 * @returns The text content of the element.
 */
function fetch(
    tagName: string,
    where: AnyNode | AnyNode[],
    recurse = false,
): string {
    return textContent(getElementsByTagName(tagName, where, recurse, 1)).trim();
}

/**
 * Adds a property to an object if it has a value.
 *
 * @param object Object to be extended.
 * @param property Property name.
 * @param tagName Tag name that contains the conditionally added property.
 * @param where Element to search for the property.
 * @param recurse Whether to recurse into child nodes.
 */
function addConditionally<T>(
    object: T,
    property: keyof T,
    tagName: string,
    where: AnyNode[],
    recurse = false,
) {
    const value = fetch(tagName, where, recurse);
    if (value) object[property] = value as unknown as T[keyof T];
}

/**
 * Checks if an element is a feed root node.
 *
 * @param value The name of the element to check.
 * @returns Whether an element is a feed root node.
 */
function isValidFeed(value: string) {
    return value === "rss" || value === "feed" || value === "rdf:RDF";
}
