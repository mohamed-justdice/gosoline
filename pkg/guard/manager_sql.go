package guard

import (
	"fmt"
	"github.com/Masterminds/squirrel"
	"github.com/applike/gosoline/pkg/cfg"
	"github.com/applike/gosoline/pkg/db"
	"github.com/applike/gosoline/pkg/mon"
	"github.com/ory/ladon"
	"github.com/thoas/go-funk"
	"time"
)

const (
	tablePolicies  = "guard_policies"
	tableSubjects  = "guard_subjects"
	tableResources = "guard_resources"
	tableActions   = "guard_actions"
)

type SqlManager struct {
	logger   mon.Logger
	dbClient db.Client
}

func NewSqlManager(config cfg.Config, logger mon.Logger) *SqlManager {
	dbClient := db.NewClient(config, logger, "default")

	return &SqlManager{
		logger:   logger,
		dbClient: dbClient,
	}
}

func (m SqlManager) Create(pol ladon.Policy) error {
	ins := squirrel.Insert(tablePolicies).Options("IGNORE").SetMap(squirrel.Eq{
		"id":          pol.GetID(),
		"description": pol.GetDescription(),
		"effect":      pol.GetEffect(),
		"updated_at":  time.Now().Format(db.FormatDateTime),
		"created_at":  time.Now().Format(db.FormatDateTime),
	})

	sql, args, err := ins.ToSql()

	if err != nil {
		return err
	}

	_, err = m.dbClient.Exec(sql, args...)

	if err != nil {
		return err
	}

	if err = m.createAssociations(pol, tableSubjects, pol.GetSubjects()); err != nil {
		return err
	}

	if err = m.createAssociations(pol, tableResources, pol.GetResources()); err != nil {
		return err
	}

	if err = m.createAssociations(pol, tableActions, pol.GetActions()); err != nil {
		return err
	}

	return nil
}

func (m SqlManager) createAssociations(pol ladon.Policy, table string, values []string) error {
	ins := squirrel.Insert(table).Options("IGNORE").Columns("id", "name")
	for _, s := range values {
		ins = ins.Values(pol.GetID(), s)
	}

	sql, args, err := ins.ToSql()

	if err != nil {
		return err
	}

	_, err = m.dbClient.Exec(sql, args...)

	return err
}

func (m SqlManager) Update(pol ladon.Policy) error {
	up := squirrel.Update(tablePolicies).Where("id = ?", pol.GetID()).SetMap(squirrel.Eq{
		"description": pol.GetDescription(),
		"effect":      pol.GetEffect(),
		"updated_at":  time.Now().Format(db.FormatDateTime),
	})

	sql, args, err := up.ToSql()

	if err != nil {
		return err
	}

	_, err = m.dbClient.Exec(sql, args...)

	if err != nil {
		return err
	}

	if err = m.updateAssociations(pol, tableResources, pol.GetResources()); err != nil {
		return err
	}

	if err = m.updateAssociations(pol, tableActions, pol.GetActions()); err != nil {
		return err
	}

	return nil
}

func (m SqlManager) updateAssociations(pol ladon.Policy, table string, values []string) error {
	err := m.deleteByIdAndTable(pol.GetID(), table)
	if err != nil {
		return err
	}

	if err = m.createAssociations(pol, table, values); err != nil {
		return err
	}

	return nil
}

func (SqlManager) Get(id string) (ladon.Policy, error) {
	panic("implement me")
}

func (m SqlManager) Delete(id string) error {
	tables := []string{tablePolicies, tableSubjects, tableResources, tableActions}

	for _, table := range tables {
		err := m.deleteByIdAndTable(id, table)

		if err != nil {
			return err
		}
	}

	return nil
}

func (m SqlManager) deleteByIdAndTable(id string, table string) error {
	del := squirrel.Delete(table).Where(squirrel.Eq{"id": id})
	sql, args, err := del.ToSql()

	if err != nil {
		m.logger.Errorf(err, "can not delete from %s", table)
		return err
	}

	_, err = m.dbClient.Exec(sql, args...)

	if err != nil {
		m.logger.Errorf(err, "can not delete from %s", table)
		return err
	}

	return nil
}

func (SqlManager) GetAll(limit, offset int64) (ladon.Policies, error) {
	panic("implement me")
}

func (m SqlManager) FindRequestCandidates(r *ladon.Request) (ladon.Policies, error) {
	return m.FindPoliciesForSubject(r.Subject)
}

func (m SqlManager) FindPoliciesForSubject(subject string) (ladon.Policies, error) {
	return m.queryPolicies(squirrel.Eq{"s.name": subject})
}

func (m SqlManager) FindPoliciesForResource(resource string) (ladon.Policies, error) {
	return m.queryPolicies(squirrel.Eq{"r.name": resource})
}

func (m SqlManager) queryPolicies(where squirrel.Eq) (ladon.Policies, error) {
	sel := squirrel.Select("p.id", "p.description", "p.effect", "s.name AS subject", "r.name AS resource", "a.name AS action")
	sel = sel.From(fmt.Sprintf("%s AS p", tablePolicies))
	sel = sel.Join(fmt.Sprintf("%s AS s ON s.id = p.id", tableSubjects))
	sel = sel.Join(fmt.Sprintf("%s AS r ON r.id = p.id", tableResources))
	sel = sel.Join(fmt.Sprintf("%s AS a ON a.id = p.id", tableActions))
	sel = sel.Where(where)
	sel = sel.OrderBy("p.id")

	sql, args, err := sel.ToSql()

	if err != nil {
		return nil, err
	}

	res, err := m.dbClient.GetResult(sql, args...)

	if err != nil {
		return nil, err
	}

	policies := map[string]*ladon.DefaultPolicy{}
	for _, row := range *res {
		if _, ok := policies[row["id"]]; !ok {
			policies[row["id"]] = &ladon.DefaultPolicy{
				ID:          row["id"],
				Description: row["description"],
				Effect:      row["effect"],
				Subjects:    make([]string, 0),
				Resources:   make([]string, 0),
				Actions:     make([]string, 0),
			}
		}

		pol := policies[row["id"]]
		pol.Subjects = append(pol.Subjects, row["subject"])
		pol.Resources = append(pol.Resources, row["resource"])
		pol.Actions = append(pol.Actions, row["action"])
	}

	unique := make(ladon.Policies, 0)
	for _, pol := range policies {
		pol.Subjects = funk.UniqString(pol.Subjects)
		pol.Resources = funk.UniqString(pol.Resources)
		pol.Actions = funk.UniqString(pol.Actions)

		unique = append(unique, pol)
	}

	return unique, nil
}
