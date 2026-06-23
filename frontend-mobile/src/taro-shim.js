import { createElement } from 'react'

export const View = ({ hoverClass, hoverStopPropagation, hoverStartTime, hoverStayTime, animation, onLongPress, onTransitionEnd, onAnimationIteration, onAnimationStart, onAnimationEnd, ...rest }) => createElement('div', rest)
export const Text = 'span'
export const Image = 'img'
export const Button = 'button'

export const ScrollView = ({ scrollY, scrollWithAnimation, enhanced, showScrollbar, scrollX, onScrollToUpper, onScrollToLower, enableBackToTop, bounces, upperThreshold, lowerThreshold, refresherEnabled, refresherThreshold, refresherTriggered, onRefresherRefresh, onRefresherRestore, onRefresherAbort, ...rest }) => createElement('div', rest)

export const Input = ({ placeholderStyle, placeholderClass, confirmType, confirmHold, cursor, selectionStart, selectionEnd, adjustPosition, holdKeyboard, cursorSpacing, focus, ...rest }) => createElement('input', rest)

export const Swiper = 'div'
export const SwiperItem = 'div'
export const Video = 'video'
export const Radio = 'input'
export const Switch = 'input'
export const Slider = 'input'
export const Textarea = 'textarea'
export const Loading = 'div'
export const Block = 'div'
export const CoverView = 'div'
export const CoverImage = 'img'
export const MovableView = 'div'
export const MoveableArea = 'div'
export const Map = 'div'
