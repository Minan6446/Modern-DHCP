import { useCallback } from 'react'
import { clampNumber, isCidr, isIpRange, isIPv4, isValidMacPrefix, normalizeMacPrefix } from '../utils/validation'
import { useI18n } from './useI18n'

export type LeaseFilterField = 'ipRange' | 'macPrefix' | 'lastSeenMinutes'

export type LeaseFilterFieldErrors = Partial<Record<LeaseFilterField, string>>

export type LeaseFilterInput = {
  ipRange?: string
  macPrefix?: string
  lastSeenMinutes?: number
}

export type LeaseFilterPayload = {
  ipRange?: string
  macPrefix?: string
  lastSeenMinutes?: number
}

export type LeaseFilterValidationResult = {
  isValid: boolean
  errors: string[]
  fieldErrors: LeaseFilterFieldErrors
  payload: LeaseFilterPayload
}

const MIN_WINDOW_MINUTES = 5
const MAX_WINDOW_MINUTES = 1440

export function useLeaseFilterGuards() {
  const { t } = useI18n()

  const validate = useCallback((input: LeaseFilterInput): LeaseFilterValidationResult => {
    const errors: string[] = []
    const fieldErrors: LeaseFilterFieldErrors = {}
    const payload: LeaseFilterPayload = {}

    const ipCandidate = input.ipRange?.trim()
    if (ipCandidate) {
      const valid = isIPv4(ipCandidate) || isCidr(ipCandidate) || isIpRange(ipCandidate)
      if (!valid) {
        const message = t('leasesFilterIpRangeError')
        errors.push(message)
        fieldErrors.ipRange = message
      } else {
        payload.ipRange = ipCandidate
      }
    }

    const macCandidate = input.macPrefix?.trim()
    if (macCandidate) {
      if (!isValidMacPrefix(macCandidate)) {
        const message = t('leasesFilterMacPrefixError')
        errors.push(message)
        fieldErrors.macPrefix = message
      } else {
        payload.macPrefix = normalizeMacPrefix(macCandidate)
      }
    }

    if (input.lastSeenMinutes !== undefined && input.lastSeenMinutes !== null) {
      if (Number.isNaN(input.lastSeenMinutes)) {
        const message = t('leasesFilterLastSeenNumberError')
        errors.push(message)
        fieldErrors.lastSeenMinutes = message
      } else if (
        input.lastSeenMinutes < MIN_WINDOW_MINUTES ||
        input.lastSeenMinutes > MAX_WINDOW_MINUTES
      ) {
        const message = t('leasesFilterLastSeenRangeError', {
          min: MIN_WINDOW_MINUTES,
          max: MAX_WINDOW_MINUTES,
        })
        errors.push(message)
        fieldErrors.lastSeenMinutes = message
      } else {
        payload.lastSeenMinutes = clampNumber(
          Math.round(input.lastSeenMinutes),
          MIN_WINDOW_MINUTES,
          MAX_WINDOW_MINUTES,
        )
      }
    }

    return {
      isValid: errors.length === 0,
      errors,
      fieldErrors,
      payload,
    }
  }, [t])

  return { validate }
}
