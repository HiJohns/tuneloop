import { View, Text } from '@tarojs/components'

const icons = {
  ArrowLeft: '‹',
  ChevronRight: '›',
  Search: '🔍',
  Heart: '♡',
  ShoppingCart: '🛒',
  MapPin: '📍',
  Clock: '⏱',
  AlertCircle: '⚠',
  Trash2: '🗑',
  Package: '📦',
  Edit2: '✏',
  Edit3: '✏',
  Calendar: '📅',
  CheckCircle: '✓',
  X: '✕',
  XCircle: '✗',
  Shield: '🛡',
  Bell: '🔔',
  Plus: '+',
  Camera: '📷',
  User: '👤',
  Phone: '📞',
  Scan: '📱',
  CreditCard: '💳',
  FileText: '📄',
  Hash: '#',
  History: '🕐',
  LogOut: '→',
  RotateCcw: '↻',
  Send: '➤',
  Upload: '↑',
  Wrench: '🔧',
  Truck: '🚚',
  Archive: '📦',
  ExternalLink: '↗',
  ClipboardList: '📋',
  AlertTriangle: '⚠',
  Key: '🔑',
  Image: '🖼',
}

function createIcon(name) {
  const IconComponent = ({ size, className, ...props }) => (
    <Text style={{ fontSize: size || 20 }} className={className} {...props}>
      {icons[name] || '?'}
    </Text>
  )
  IconComponent.displayName = name
  return IconComponent
}

export const ArrowLeft = createIcon('ArrowLeft')
export const ChevronRight = createIcon('ChevronRight')
export const Search = createIcon('Search')
export const Heart = createIcon('Heart')
export const ShoppingCart = createIcon('ShoppingCart')
export const MapPin = createIcon('MapPin')
export const Clock = createIcon('Clock')
export const AlertCircle = createIcon('AlertCircle')
export const Trash2 = createIcon('Trash2')
export const Package = createIcon('Package')
export const Edit2 = createIcon('Edit2')
export const Edit3 = createIcon('Edit3')
export const Calendar = createIcon('Calendar')
export const CheckCircle = createIcon('CheckCircle')
export const X = createIcon('X')
export const XCircle = createIcon('XCircle')
export const Shield = createIcon('Shield')
export const Bell = createIcon('Bell')
export const Plus = createIcon('Plus')
export const Camera = createIcon('Camera')
export const User = createIcon('User')
export const Phone = createIcon('Phone')
export const Scan = createIcon('Scan')
export const CreditCard = createIcon('CreditCard')
export const FileText = createIcon('FileText')
export const Hash = createIcon('Hash')
export const History = createIcon('History')
export const LogOut = createIcon('LogOut')
export const RotateCcw = createIcon('RotateCcw')
export const Send = createIcon('Send')
export const Upload = createIcon('Upload')
export const Wrench = createIcon('Wrench')
export const Truck = createIcon('Truck')
export const Archive = createIcon('Archive')
export const ExternalLink = createIcon('ExternalLink')
export const ClipboardList = createIcon('ClipboardList')
export const AlertTriangle = createIcon('AlertTriangle')
export const Key = createIcon('Key')
export const Image = createIcon('Image')
