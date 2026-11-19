import { array, nullable, number, object, string, type Infer } from './parse';

export const Ranked = object({ name: string, sessions: number, tokens: number, share: number });
export type Ranked = Infer<typeof Ranked>;

export const Day = object({
  date: string,
  sessions: number,
  tokens: number,
  cost: number,
  activeUsers: number,
  activeHosts: number,
});

export const Figures = object({
  sessions: number,
  tokensIn: number,
  tokensOut: number,
  costUsd: number,
  activeUsers: number,
  activeHosts: number,
  daily: array(Day),
  models: array(Ranked),
  agents: array(Ranked),
});

export const TeamData = object({
  from: string,
  to: string,
  org: Figures,
  you: nullable(Figures),
});
export type TeamData = Infer<typeof TeamData>;
