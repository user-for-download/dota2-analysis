SELECT *
FROM teams
JOIN team_rating ON teams.team_id = team_rating.team_id
WHERE TRUE