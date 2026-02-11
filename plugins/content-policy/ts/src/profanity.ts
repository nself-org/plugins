/**
 * Built-in profanity word list
 * Common profane and offensive words for content filtering
 */

export const PROFANITY_WORDS = new Set([
  // Common profanity (mild to moderate)
  'damn',
  'hell',
  'crap',
  'piss',
  'bastard',
  'bitch',
  'ass',
  'asshole',
  'dick',
  'cock',
  'pussy',
  'shit',
  'fuck',
  'fucking',
  'motherfucker',
  'bullshit',
  'horseshit',
  'dickhead',
  'douche',
  'douchebag',
  'jackass',
  'prick',
  'twat',
  'wanker',

  // Slurs and offensive terms (removed for safety - implementers should add appropriate terms)
  // Note: This is a minimal list. Production systems should use comprehensive,
  // context-aware filtering solutions like Perspective API or similar services.
]);
