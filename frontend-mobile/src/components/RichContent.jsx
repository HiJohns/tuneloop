import { View, Text, RichText, Image } from '@tarojs/components'
import { env, previewImage } from '../platform'
import { normalizeContentUrls, injectResponsiveImageStyle, splitRichTextImages } from '../utils/content'

// Shared rich-content renderer for H5 and weapp (single source).
//
// Weapp native <rich-text> isolates its internal nodes from page CSS and
// cannot attach tap handlers to them (#1907). Renderer therefore:
//   1. normalizes relative asset URLs against the API origin,
//   2. injects inline max-width so images fit the screen width,
//   3. splits image blocks out and renders each as a Taro <Image> that,
//      when tapped, opens the platform image preview (multi-image swipe).
//
// New callers only need to pass `html` — origin derivation, URL
// normalization, image sizing and tap-to-preview all live here.
export default function RichContent({ html, origin: originOverride }) {
  const origin = originOverride || (env.apiBaseUrl || '').replace(/\/api\/?$/, '')
  const normalized = normalizeContentUrls(html, origin)

  // Plain text (no HTML tags) → styled Text, matching prior pages' fallbacks.
  if (!/<[a-z][\s\S]*>/i.test(normalized)) {
    return (
      <Text className="text-sm text-zinc-600 block" style={{ lineHeight: '1.6', whiteSpace: 'pre-wrap' }}>
        {normalized}
      </Text>
    )
  }

  const segments = splitRichTextImages(injectResponsiveImageStyle(normalized))
  const imageUrls = segments.filter(s => s.type === 'img').map(s => s.src)

  return (
    <View>
      {segments.map((seg, i) =>
        seg.type === 'img' ? (
          <Image
            key={i}
            src={seg.src}
            mode="widthFix"
            style={{ width: '100%', display: 'block', marginTop: 4, marginBottom: 4 }}
            onClick={() => {
              previewImage({ urls: imageUrls, current: seg.src })
            }}
          />
        ) : (
          <View key={i}>
            <RichText nodes={seg.content} />
          </View>
        )
      )}
    </View>
  )
}