import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import { Icon } from './shared'
import './themed-select.css'

export type SelectOption = { value: string; label: string }

export function ThemedSelect({label, name, value, options, onChange, disabled}: {
  label: string
  name?: string
  value: string
  options: readonly SelectOption[]
  onChange: (value: string) => void
  disabled?: boolean
}) {
  const id = useId()
  const root = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const list = useRef<HTMLUListElement>(null)
  const [menu, setMenu] = useState<{active: number; above: boolean} | null>(null)
  const typed = useRef({text: '', at: 0})
  const selected = options.findIndex(option => option.value === value)
  const unavailable = disabled || options.length === 0
  const expanded = !unavailable && menu !== null

  useEffect(() => {
    if (unavailable) setMenu(null)
  }, [unavailable])

  useEffect(() => {
    if (!expanded) return
    const closeOutside = (event: PointerEvent) => {
      if (event.target instanceof Node && !root.current?.contains(event.target)) setMenu(null)
    }
    document.addEventListener('pointerdown', closeOutside)
    return () => document.removeEventListener('pointerdown', closeOutside)
  }, [expanded])

  useEffect(() => {
    if (expanded) list.current?.querySelector('[data-active]')?.scrollIntoView({block: 'nearest'})
  }, [expanded, menu?.active])

  function open(active = Math.max(0, selected)) {
    const rect = trigger.current?.getBoundingClientRect()
    const above = !!rect && window.innerHeight - rect.bottom < 240 && rect.top > window.innerHeight - rect.bottom
    typed.current = {text: '', at: 0}
    setMenu({active, above})
  }

  function choose(index: number) {
    const option = options[index]
    if (option && option.value !== value) onChange(option.value)
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
      if (menu) choose(menu.active)
      else open()
    } else if (menu && key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      setMenu(null)
    } else if (menu && key === 'Tab') {
      choose(menu.active)
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
    <label id={id + '-label'} htmlFor={id + '-control'}>{label}</label>
    <button className="button themed-select-trigger" ref={trigger} type="button" id={id + '-control'}
      role="combobox" aria-labelledby={id + '-label'} aria-controls={expanded ? id + '-options' : undefined}
      aria-expanded={expanded} aria-activedescendant={expanded ? id + '-option-' + menu.active : undefined}
      disabled={unavailable} onKeyDown={keydown}
      onBlur={event => { if (event.relatedTarget && !root.current?.contains(event.relatedTarget)) setMenu(null) }}
      onClick={() => { if (expanded) setMenu(null); else open() }}>
      <span>{options[selected]?.label ?? value}</span><Icon name="ChevronDown" size={16}/>
    </button>
    {name && <input type="hidden" name={name} value={value} disabled={unavailable}/>}
    {expanded && <ul className="themed-select-menu" ref={list} role="listbox" id={id + '-options'}
      data-placement={menu.above ? 'above' : 'below'} aria-labelledby={id + '-label'}>
      {options.map((option, index) => <li key={option.value} id={id + '-option-' + index} role="option"
        aria-selected={index === selected} data-active={index === menu.active ? '' : undefined}
        onMouseDown={event => event.preventDefault()}
        onClick={() => { choose(index); trigger.current?.focus() }}>
        <span>{option.label}</span>{index === selected && <Icon name="Check" size={15}/>}
      </li>)}
    </ul>}
  </div>
}
