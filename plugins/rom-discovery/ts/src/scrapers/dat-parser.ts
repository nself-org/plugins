/**
 * Shared Logiqx XML DAT Parser
 * Parses No-Intro and Redump DAT files in the standard Logiqx XML DTD format.
 * Uses regex-based parsing to avoid adding external XML dependencies.
 */

import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('rom-discovery:dat-parser');

// =============================================================================
// Types
// =============================================================================

export interface DatHeader {
  name: string;
  description: string;
  version?: string;
  date?: string;
  author?: string;
}

export interface DatRomEntry {
  name: string;
  size?: number;
  crc32?: string;
  md5?: string;
  sha1?: string;
  serial?: string;
  status?: string;
}

export interface DatGameEntry {
  name: string;
  description?: string;
  roms: DatRomEntry[];
  cloneOf?: string;
  year?: string;
  manufacturer?: string;
}

export interface DatFile {
  header: DatHeader;
  games: DatGameEntry[];
}

// =============================================================================
// Region / Revision / Language Extraction
// =============================================================================

const REGION_PATTERNS: Record<string, string> = {
  'USA': 'USA',
  'US': 'USA',
  'United States': 'USA',
  'Europe': 'Europe',
  'EU': 'Europe',
  'Japan': 'Japan',
  'JP': 'Japan',
  'World': 'World',
  'Asia': 'Asia',
  'Australia': 'Australia',
  'Brazil': 'Brazil',
  'Canada': 'Canada',
  'China': 'China',
  'France': 'France',
  'Germany': 'Germany',
  'Italy': 'Italy',
  'Korea': 'Korea',
  'Netherlands': 'Netherlands',
  'Spain': 'Spain',
  'Sweden': 'Sweden',
  'Russia': 'Russia',
  'Taiwan': 'Taiwan',
  'Hong Kong': 'Hong Kong',
  'Scandinavia': 'Scandinavia',
};

const LANGUAGE_MAP: Record<string, string> = {
  'En': 'English',
  'Ja': 'Japanese',
  'Fr': 'French',
  'De': 'German',
  'Es': 'Spanish',
  'It': 'Italian',
  'Pt': 'Portuguese',
  'Nl': 'Dutch',
  'Sv': 'Swedish',
  'No': 'Norwegian',
  'Da': 'Danish',
  'Fi': 'Finnish',
  'Ko': 'Korean',
  'Zh': 'Chinese',
  'Ru': 'Russian',
  'Pl': 'Polish',
  'Cs': 'Czech',
  'Hu': 'Hungarian',
};

/**
 * Extract region from a game/ROM name using parenthetical tags.
 * E.g., "Super Mario Bros. (USA)" -> "USA"
 *       "Final Fantasy (Japan, Europe)" -> "Japan"  (first match)
 */
export function extractRegion(name: string): string | null {
  // Match parenthetical groups
  const parenGroups = name.match(/\(([^)]+)\)/g);
  if (!parenGroups) return null;

  for (const group of parenGroups) {
    const content = group.slice(1, -1); // Remove parens
    // Split on comma to handle "(USA, Europe)" patterns
    const parts = content.split(',').map(p => p.trim());

    for (const part of parts) {
      const mapped = REGION_PATTERNS[part];
      if (mapped) return mapped;
    }
  }

  return null;
}

/**
 * Extract revision info from a game/ROM name.
 * E.g., "Game (Rev A)" -> "Rev A"
 *       "Game (v1.1)" -> "v1.1"
 *       "Game (Rev 2)" -> "Rev 2"
 */
export function extractRevision(name: string): string | null {
  const revMatch = name.match(/\((Rev\s+[A-Za-z0-9.]+)\)/i);
  if (revMatch) return revMatch[1];

  const versionMatch = name.match(/\((v[0-9]+(?:\.[0-9]+)*)\)/i);
  if (versionMatch) return versionMatch[1];

  const versionFullMatch = name.match(/\((Version\s+[A-Za-z0-9.]+)\)/i);
  if (versionFullMatch) return versionFullMatch[1];

  return null;
}

/**
 * Extract languages from a game/ROM name.
 * E.g., "Game (En,Fr,De)" -> ["English", "French", "German"]
 *       "Game (En)" -> ["English"]
 */
export function extractLanguages(name: string): string[] {
  const languages: string[] = [];

  // Match parenthetical groups that look like language codes
  const parenGroups = name.match(/\(([^)]+)\)/g);
  if (!parenGroups) return languages;

  for (const group of parenGroups) {
    const content = group.slice(1, -1);
    const parts = content.split(',').map(p => p.trim());

    // Check if this group contains language codes
    const allAreLanguages = parts.length > 0 && parts.every(
      p => LANGUAGE_MAP[p] !== undefined
    );

    if (allAreLanguages) {
      for (const part of parts) {
        const lang = LANGUAGE_MAP[part];
        if (lang && !languages.includes(lang)) {
          languages.push(lang);
        }
      }
    }
  }

  return languages;
}

// =============================================================================
// XML Parsing Utilities
// =============================================================================

/**
 * Decode XML entities in a string.
 */
function unescapeXml(str: string): string {
  return str
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'");
}

/**
 * Extract an XML attribute value by name from a tag string.
 * Handles both single and double quoted attributes.
 */
function getAttr(tag: string, attrName: string): string | undefined {
  // Match: attrName="value" or attrName='value'
  const regex = new RegExp(`${attrName}\\s*=\\s*(?:"([^"]*)"|'([^']*)')`, 'i');
  const match = tag.match(regex);
  if (!match) return undefined;
  const raw = match[1] ?? match[2];
  return raw !== undefined ? unescapeXml(raw) : undefined;
}

/**
 * Extract text content between opening and closing tags.
 * E.g., "<name>My Game</name>" -> "My Game"
 */
function getTagContent(xml: string, tagName: string): string | undefined {
  const regex = new RegExp(`<${tagName}(?:\\s[^>]*)?>([\\s\\S]*?)</${tagName}>`, 'i');
  const match = xml.match(regex);
  if (!match) return undefined;
  return unescapeXml(match[1].trim());
}

// =============================================================================
// Main DAT Parser
// =============================================================================

/**
 * Parse a Logiqx XML DAT file content into a structured DatFile object.
 *
 * The Logiqx format structure:
 * ```xml
 * <datafile>
 *   <header>
 *     <name>...</name>
 *     <description>...</description>
 *     <version>...</version>
 *     <date>...</date>
 *     <author>...</author>
 *   </header>
 *   <game name="..." cloneof="...">
 *     <description>...</description>
 *     <year>...</year>
 *     <manufacturer>...</manufacturer>
 *     <rom name="..." size="..." crc="..." md5="..." sha1="..." serial="..." status="..."/>
 *   </game>
 * </datafile>
 * ```
 */
export function parseDatXml(xmlContent: string): DatFile {
  logger.debug('Parsing DAT XML content', { length: xmlContent.length });

  // Parse header
  const headerBlock = xmlContent.match(/<header>([\s\S]*?)<\/header>/i);
  const headerXml = headerBlock ? headerBlock[1] : '';

  const header: DatHeader = {
    name: getTagContent(headerXml, 'name') ?? 'Unknown',
    description: getTagContent(headerXml, 'description') ?? '',
    version: getTagContent(headerXml, 'version'),
    date: getTagContent(headerXml, 'date'),
    author: getTagContent(headerXml, 'author'),
  };

  logger.debug('Parsed DAT header', { name: header.name, version: header.version });

  // Parse game entries
  // Match each <game ...>...</game> block (or self-closing <game .../>)
  const games: DatGameEntry[] = [];
  const gameRegex = /<game\s([^>]*?)>([\s\S]*?)<\/game>/gi;
  let gameMatch: RegExpExecArray | null;

  while ((gameMatch = gameRegex.exec(xmlContent)) !== null) {
    const gameAttrs = gameMatch[1];
    const gameBody = gameMatch[2];

    const gameName = getAttr(`<game ${gameAttrs}>`, 'name');
    if (!gameName) continue;

    const cloneOf = getAttr(`<game ${gameAttrs}>`, 'cloneof');

    const game: DatGameEntry = {
      name: gameName,
      description: getTagContent(gameBody, 'description'),
      roms: [],
      cloneOf: cloneOf,
      year: getTagContent(gameBody, 'year'),
      manufacturer: getTagContent(gameBody, 'manufacturer'),
    };

    // Parse rom entries within this game
    // Match both self-closing <rom .../> and <rom ...></rom>
    const romRegex = /<rom\s([^>]*?)(?:\/>|>[^<]*<\/rom>)/gi;
    let romMatch: RegExpExecArray | null;

    while ((romMatch = romRegex.exec(gameBody)) !== null) {
      const romTag = `<rom ${romMatch[1]}/>`;

      const romName = getAttr(romTag, 'name');
      if (!romName) continue;

      const sizeStr = getAttr(romTag, 'size');

      const rom: DatRomEntry = {
        name: romName,
        size: sizeStr !== undefined ? parseInt(sizeStr, 10) : undefined,
        crc32: getAttr(romTag, 'crc'),
        md5: getAttr(romTag, 'md5'),
        sha1: getAttr(romTag, 'sha1'),
        serial: getAttr(romTag, 'serial'),
        status: getAttr(romTag, 'status'),
      };

      // Validate parsed size
      if (rom.size !== undefined && isNaN(rom.size)) {
        rom.size = undefined;
      }

      game.roms.push(rom);
    }

    games.push(game);
  }

  logger.info('DAT file parsed', {
    headerName: header.name,
    totalGames: games.length,
    totalRoms: games.reduce((sum, g) => sum + g.roms.length, 0),
  });

  return { header, games };
}
