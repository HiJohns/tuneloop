import { MapPin } from 'lucide-react'

export default function SiteSelector({ sites, selectedSite, onSelect }) {
  return (
    <div className="space-y-3">
      <select
        value={selectedSite?.id || ''}
        onChange={(e) => {
          const site = sites.find(s => s.id === parseInt(e.target.value))
          onSelect(site)
        }}
        className="w-full p-3 border rounded-lg"
      >
        <option value="">选择附近网点</option>
        {sites.map(site => (
          <option key={site.id} value={site.id}>
            {site.name} - 距离 {site.distance}km
          </option>
        ))}
      </select>

      {selectedSite && (
        <div className="flex items-center gap-2 p-3 bg-blue-50 rounded-lg">
          <MapPin className="text-blue-500 flex-shrink-0" size={20} />
          <div>
            <p className="font-medium text-blue-800">{selectedSite.name}</p>
            <p className="text-blue-600 text-sm">{selectedSite.address}</p>
          </div>
        </div>
      )}
    </div>
  )
}