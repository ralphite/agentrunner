/**
 * Compatibility entry point.
 *
 * Composer is a feature controller: it owns app services, store access,
 * persistence, and async orchestration. Keep existing imports stable while the
 * production implementation lives under features/composer.
 */
export {
  Composer,
  ComposerView,
  GoalLoopLauncher,
  type ComposerProps,
  type SessionActions,
} from "../features/composer/ComposerController";
