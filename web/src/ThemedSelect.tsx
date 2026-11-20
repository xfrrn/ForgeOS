import { Fragment, useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'
import { Icon } from './shared'
import './themed-select.css'

export type SelectOption = { value: string; label: string; group?: string }

export function ThemedSelect({label, name, value, options, onChange, disabled, hideLabel = false}: {
  label: string
  name?: string
  value: string
  options: readonly SelectOption[]
  onChange: (value: string) => void
  hideLabel?: boolean
  disabled?: boolean
}) {
  const id = useId()
  const root = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const list = useRef<HTMLUListElement>(null)
  const [menu, setMenu] = useState<{active: number} | null>(null)
  const [position, setPosition] = useState({left: 0, top: 0, width: 0, maxHeight: 0, above: false})
  const typed = useRef({text: '', at: 0})
  const selected = options.findIndex(option => option.value === value)
  const unavailable = disabled || options.length === 0
  const expanded = !unavailable && menu !== null

  useEffect(() => {
    if (unavailable) setMenu(null)
  }, [unavailable])

  useLayoutEffect(() => {
    if (!expanded || !trigger.current) return
    const button = trigger.current
    function place() {
      const rect = button.getBoundingClientRect()
      const height = document.documentElement.clientHeight
      const viewport = document.documentElement.clientWidth
      if (!rect.width || rect.bottom <= 0 || rect.top >= height || button.matches(':disabled')) {
        setMenu(null)
        return
      }
      const above = height - rect.bottom < 240 && rect.top > height - rect.bottom
      const width = Math.min(Math.max(168, rect.width), viewport - 16)
      setPosition({
        left: Math.max(8, Math.min(rect.left, viewport - width - 8)), width, above,
        top: above ? rect.top - 4 : rect.bottom + 4,
        maxHeight: Math.max(0, Math.min(288, (above ? rect.top : height - rect.bottom) - 12))
      })
    }
    function reposition(event: Event) {
      if (!(event.target instanceof Node && list.current?.contains(event.target))) place()
    }
    place()
    const observer = new ResizeObserver(place)
    observer.observe(button)
    window.addEventListener('resize', place)
    document.addEventListener('scroll', reposition, true)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', place)
      document.removeEventListener('scroll', reposition, true)
    }
  }, [expanded])

  useEffect(() => {
    if (!expanded) return
    const closeOutside = (event: PointerEvent) => {
      if (event.target instanceof Node && !root.current?.contains(event.target) && !list.current?.contains(event.target)) setMenu(null)
    }
    document.addEventListener('pointerdown', closeOutside)
    return () => document.removeEventListener('pointerdown', closeOutside)
  }, [expanded])

  const active = Math.min(menu?.active ?? 0, options.length - 1)
  useLayoutEffect(() => {
    if (!expanded || !position.width) return
    const panel = list.current
    const row = panel?.querySelector<HTMLElement>('[data-active]')
    if (!panel || !row) return
    const item = row.getBoundingClientRect(), bounds = panel.getBoundingClientRect()
    if (item.top < bounds.top) panel.scrollTop += item.top - bounds.top - 4
    else if (item.bottom > bounds.bottom) panel.scrollTop += item.bottom - bounds.bottom + 4
  }, [expanded, active, position.width, position.maxHeight])

  function open(active = Math.max(0, selected)) {
    typed.current = {text: '', at: 0}
    setMenu({active})
  }

  function choose(index: number) {
    const option = options[index]
    if (!trigger.current?.matches(':disabled') && option && option.value !== value) onChange(option.value)
    setMenu(null)
  }

  function keydown(event: KeyboardEvent<HTMLButtonElement>) {
    if (unavailable) return
    const {key} = event
    if (key === 'ArrowDown' || key === 'ArrowUp' || key === 'Home' || key === 'End') {
      event.preventDefault()
      const last = options.length - 1
      const active = key === 'Home' ? 0 : key === 'End' ? last :
        menu ? Math.max(0, Math.min(last, menu.active + (key === 'ArrowDown' ? 1 : -1))) :
          selected < 0 && key === 'ArrowUp' ? last : Math.max(0, selected)
      if (menu) setMenu({...menu, active})
      else open(active)
    } else if (key === 'Enter' || key === ' ') {
      event.preventDefault()
      if (menu) choose(active)
      else open()
    } else if (menu && key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      setMenu(null)
    } else if (menu && key === 'Tab') {
      choose(active)
    } else if (key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey && !event.nativeEvent.isComposing) {
      const now = Date.now()
      const character = key.toLocaleLowerCase()
      const previous = now - typed.current.at < 600 ? typed.current.text : ''
      const query = previous === character ? character : previous + character
      const start = query.length === 1 ? (menu?.active ?? selected) + 1 : 0
      for (let offset = 0; offset < options.length; offset++) {
        const index = (start + offset) % options.length
        if (!options[index].label.toLocaleLowerCase().startsWith(query)) continue
        event.preventDefault()
        if (menu) setMenu({...menu, active: index})
        else open(index)
        break
      }
      typed.current = {text: query, at: now}
    }
  }

  return <div className="themed-select" ref={root}>
    <label className={hideLabel ? "sr-only" : undefined} id={id + '-label'} htmlFor={id + '-control'}>{label}</label>
    <button className="button themed-select-trigger" ref={trigger} type="button" id={id + '-control'}
      role="combobox" aria-labelledby={id + '-label'} aria-controls={expanded ? id + '-options' : undefined}
      aria-expanded={expanded} aria-activedescendant={expanded ? id + '-option-' + active : undefined}
      disabled={unavailable} onKeyDown={keydown}
      onBlur={event => { if (!root.current?.contains(event.relatedTarget)) setMenu(null) }}
      onClick={() => { if (expanded) setMenu(null); else open() }}>
      <span>{options[selected]?.label ?? value}</span><Icon name="ChevronDown" size={16}/>
    </button>
    {name && <input type="hidden" name={name} value={value} disabled={unavailable}/>}
    {expanded && createPortal(<ul className="themed-select-menu" ref={list} role="listbox" id={id + '-options'}
      style={{left:position.left, top:position.top, width:position.width, maxHeight:position.maxHeight}}
      data-placement={position.above ? 'above' : 'below'} aria-labelledby={id + '-label'}>
      {options.map((option, index) => <Fragment key={option.value}>
        {option.group && options[index-1]?.group !== option.group &&
          <li aria-hidden="true" className="themed-select-group">{option.group}</li>}
        <li id={id + '-option-' + index} role="option"
          aria-label={option.group ? option.group + ' · ' + option.label : undefined}
          aria-selected={index === selected} data-active={index === active ? '' : undefined}
          onMouseDown={event => event.preventDefault()}
          onClick={() => { choose(index); trigger.current?.focus({preventScroll:true}) }}>
          <span>{option.label}</span>{index === selected && <Icon name="Check" size={15}/>}
        </li>
      </Fragment>)}
    </ul>, document.body)}
  </div>
}
