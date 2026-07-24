// Compatibility boundary for existing imports and Storybook stories.
// Runtime orchestration and state ownership live in features/session.
export {
  GoalBanner,
  ProgressSummary,
  SessionFeature as SessionView,
  isSessionNotFound,
  isValidSessionId,
} from "../features/session/SessionFeature";
