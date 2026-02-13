export const languageToCountryMap: Record<string, string> = {
  'af': 'ZA', 'al': 'AL', 'am': 'ET', 'ar': 'SA', 'az': 'AZ',
  'be': 'BY', 'bg': 'BG', 'bn': 'BD', 'bs': 'BA', 'ca': 'ES',
  'ceb': 'PH', 'cs': 'CZ', 'cv': 'RU', 'da': 'DK', 'de': 'DE',
  'dk': 'DK', 'el': 'GR', 'en': 'GB', 'eo': 'UN', 'es': 'ES',
  'es-419': 'MX', 'et': 'EE', 'eu': 'ES', 'fa': 'IR', 'fi': 'FI',
  'fil': 'PH', 'fo': 'FO', 'fr': 'FR', 'ga': 'IE', 'gl': 'ES',
  'gn': 'PY', 'gu': 'IN', 'ha': 'NG', 'he': 'IL', 'hi': 'IN',
  'hr': 'HR', 'ht': 'HT', 'hu': 'HU', 'hy': 'AM', 'id': 'ID',
  'ig': 'NG', 'is': 'IS', 'it': 'IT', 'ja': 'JP', 'jv': 'ID',
  'ka': 'GE', 'kk': 'KZ', 'km': 'KH', 'kn': 'IN', 'ko': 'KR',
  'ku': 'IQ', 'ky': 'KG', 'la': 'VA', 'lb': 'LU', 'lo': 'LA',
  'lt': 'LT', 'lv': 'LV', 'mg': 'MG', 'mi': 'NZ', 'mk': 'MK',
  'ml': 'IN', 'mn': 'MN', 'mo': 'MD', 'mr': 'IN', 'ms': 'MY',
  'my': 'MM', 'ne': 'NP', 'nl': 'NL', 'no': 'NO', 'none': 'UN',
  'ny': 'MW', 'other': 'UN', 'pa': 'IN', 'pl': 'PL', 'po': 'PT',
  'ps': 'AF', 'pt': 'PT', 'pt-br': 'BR', 'rm': 'CH', 'ro': 'RO',
  'ru': 'RU', 'sd': 'PK', 'sh': 'RS', 'si': 'LK', 'sk': 'SK',
  'sl': 'SI', 'sm': 'WS', 'sn': 'ZW', 'so': 'SO', 'sq': 'AL',
  'sr': 'RS', 'st': 'ZA', 'sv': 'SE', 'sw': 'TZ', 'ta': 'IN',
  'te': 'IN', 'tg': 'TJ', 'th': 'TH', 'ti': 'ER', 'tk': 'TM',
  'tl': 'PH', 'to': 'TO', 'tr': 'TR', 'uk': 'UA', 'ur': 'PK',
  'uz': 'UZ', 'vi': 'VN', 'yo': 'NG', 'zh': 'CN', 'zh-hans': 'CN',
  'zh-hant': 'TW', 'zu': 'ZA', 'all': 'UN',
}

export function getCountryCodeForLanguage(languageCode: string): string {
  return languageToCountryMap[languageCode.toLowerCase()] ?? 'UN'
}

export function langToFlagClass(lang: string): string {
  const cc = getCountryCodeForLanguage(lang).toLowerCase()
  return `fi fi-${cc}`
}
