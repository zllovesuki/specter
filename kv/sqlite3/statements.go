package sqlite3

import "database/sql"

// statements holds prepared statements for fixed-shape queries.
// Reader statements are prepared on s.reader; writer statements on s.writer.
// For transactional writes, callers use tx.StmtContext to bind a prepared
// statement into the active transaction.
type statements struct {
	// -- reader --
	simpleGet      *sql.Stmt
	prefixContains *sql.Stmt
	prefixList     *sql.Stmt
	listKeys       *sql.Stmt
	rangeKeysNorm  *sql.Stmt
	rangeKeysWrap  *sql.Stmt

	// -- writer --
	simplePut    *sql.Stmt
	simpleDel    *sql.Stmt
	prefixAppend *sql.Stmt
	prefixRemove *sql.Stmt
	prefixCount  *sql.Stmt

	leaseAcquire *sql.Stmt
	leaseRenew   *sql.Stmt
	leaseRelease *sql.Stmt
	leaseGet     *sql.Stmt
	leaseImport  *sql.Stmt

	trackerLookup *sql.Stmt
	trackerInsert *sql.Stmt
	trackerUpdate *sql.Stmt
	trackerDelete *sql.Stmt

	// export uses the reader inside a read tx
	exportSimpleGet  *sql.Stmt
	exportPrefixList *sql.Stmt
	exportLeaseGet   *sql.Stmt

	all []*sql.Stmt
}

func prepareStatements(reader, writer *sql.DB) (_ *statements, err error) {
	s := &statements{}
	defer func() {
		if err != nil {
			s.close()
		}
	}()

	type preparedStatement struct {
		db     *sql.DB
		query  string
		target **sql.Stmt
	}

	defs := []preparedStatement{
		{reader, querySimpleGet, &s.simpleGet},
		{reader, queryPrefixContains, &s.prefixContains},
		{reader, queryPrefixList, &s.prefixList},
		{reader, queryListKeys, &s.listKeys},
		{reader, queryRangeKeysNorm, &s.rangeKeysNorm},
		{reader, queryRangeKeysWrap, &s.rangeKeysWrap},
		{writer, querySimplePut, &s.simplePut},
		{writer, querySimpleDel, &s.simpleDel},
		{writer, queryPrefixAppend, &s.prefixAppend},
		{writer, queryPrefixRemove, &s.prefixRemove},
		{writer, queryPrefixCount, &s.prefixCount},
		{writer, queryLeaseAcquire, &s.leaseAcquire},
		{writer, queryLeaseRenew, &s.leaseRenew},
		{writer, queryLeaseRelease, &s.leaseRelease},
		{writer, queryLeaseGet, &s.leaseGet},
		{writer, queryLeaseImport, &s.leaseImport},
		{writer, queryTrackerLookup, &s.trackerLookup},
		{writer, queryTrackerInsert, &s.trackerInsert},
		{writer, queryTrackerUpdate, &s.trackerUpdate},
		{writer, queryTrackerDelete, &s.trackerDelete},
		{reader, queryLeaseGet, &s.exportLeaseGet},
	}

	for _, def := range defs {
		st, prepErr := def.db.Prepare(def.query)
		if prepErr != nil {
			err = prepErr
			return nil, err
		}
		*def.target = st
		s.all = append(s.all, st)
	}

	s.exportSimpleGet = s.simpleGet
	s.exportPrefixList = s.prefixList

	return s, nil
}

func (s *statements) close() {
	if s == nil {
		return
	}
	for _, st := range s.all {
		if st != nil {
			_ = st.Close()
		}
	}
	s.all = nil
}
