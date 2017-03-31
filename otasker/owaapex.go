// owaapex
package otasker

func NewOwaApexProcRunner() func() OracleTasker {
	return func() OracleTasker {
		return newTaskerIntf(apexEvalSessionID, apexMain, apexGetRestChunk, apexKillSession, apexFileUpload)
	}
}

func NewOwaApexProcTasker() func() oracleTasker {
	return func() oracleTasker {
		return newTasker(apexEvalSessionID, apexMain, apexGetRestChunk, apexKillSession, apexFileUpload)
	}
}

const (
	apexEvalSessionID = `select kill_session.get_current_session_id from dual`
	apexMain          = `
Declare
  rc__ number(2,0);
  l_num_params number;
  l_param_name owa.vc_arr;
  l_param_val owa.vc_arr;
  l_num_ext_params number;
  l_ext_param_name owa.vc_arr;
  l_ext_param_val owa.vc_arr;
  l_package_name varchar(240);
%s
Begin
%s 
  /* >> Инициализация параметров */
%s
  /* << Инициализация параметров */
  %s
  owa.init_cgi_env(l_num_params, l_param_name, l_param_val);
%s
  %s(%s);
  %s
  if (wpg_docload.is_file_download) then
    rc__ := 1;
    :content__ := '';
    :bNextChunkExists := 0;
    declare
      l_doc_info varchar2(32000);
      l_lob blob := :lob__;
      l_bfile bfile;
    begin
      wpg_docload.get_download_file(l_doc_info);
      if l_doc_info='B' then
        apex_public_user.hrslt.GET_INFO(:ContentType ,:ContentLength,:CustomHeaders);
        wpg_docload.get_download_blob(l_lob);
        :lob__ := l_lob;
      elsif l_doc_info='F' then
        apex_public_user.hrslt.GET_INFO(:ContentType ,:ContentLength,:CustomHeaders);
        wpg_docload.get_download_bfile(l_bfile);
        DBMS_LOB.LOADFROMFILE(l_lob, l_bfile, DBMS_LOB.getLength(l_bfile));
        :lob__ := l_lob;
      else
        declare
          l_len number;
          l_rest varchar2(32000) := l_doc_info;
          l_fn varchar2(32000);
          l_ct varchar2(4000);
          p_doctable varchar2(32000);
          sql_stmt varchar2(32000);
          cursor_handle INTEGER;
          retval INTEGER;
        begin
          l_len :=to_number('0'||substr(l_doc_info,1, instr(l_doc_info,'X', 1)-1));
          l_fn := substr(l_doc_info,instr(l_doc_info,'X', 1)+1, l_len);
          p_doctable := owa_util.get_cgi_env('DOCUMENT_TABLE');
          IF (p_doctable IS NULL) THEN
             p_doctable := 'wwv_document';
          END IF;

          sql_stmt := 'select nvl(MIME_TYPE,CONTENT_TYPE), blob_content  from '||p_doctable||
            ' where NAME=:docname';
          cursor_handle := dbms_sql.open_cursor;
          dbms_sql.parse(cursor_handle, sql_stmt, dbms_sql.v7);

          dbms_sql.define_column(cursor_handle, 1, l_ct, 128);
          dbms_sql.define_column(cursor_handle, 2, l_lob);
          dbms_sql.bind_variable(cursor_handle, ':docname', l_fn);

          retval := dbms_sql.execute_and_fetch(cursor_handle,TRUE);

          dbms_sql.column_value(cursor_handle, 1, l_ct);
          dbms_sql.column_value(cursor_handle, 2, l_lob);
          dbms_sql.close_cursor(cursor_handle);
          :ContentType := l_ct;
          :ContentLength := dbms_lob.getlength(l_lob);
          :CustomHeaders := '';
          :lob__ := l_lob;

        end;
      end if;
    end;
    commit;
    dbms_session.modify_package_state(dbms_session.reinitialize);
  else
    rc__ := 0;
    commit;
    apex_public_user.hrslt.GET_INFO(:ContentType ,:ContentLength,:CustomHeaders);
    :content__ := apex_public_user.hrslt.GET32000(:bNextChunkExists);
    if :bNextChunkExists = 0 then
      dbms_session.modify_package_state(dbms_session.reinitialize);
    end if;
  end if;
  commit;
  :rc__ := rc__;
  :sqlerrcode := 0;
  :sqlerrm := '';
  :sqlerrtrace := '';
exception
  when others then
    rollback;
    :sqlerrcode := SQLCODE;
    :sqlerrm := sqlerrm;
    :sqlerrtrace := DBMS_UTILITY.FORMAT_ERROR_BACKTRACE();
end;`

	apexGetRestChunk = `begin
  :Data:=apex_public_user.hrslt.GET32000(:bNextChunkExists);
  if :bNextChunkExists = 0 then
    dbms_session.modify_package_state(dbms_session.reinitialize);
  end if;
  commit;
  :sqlerrcode := 0;
  :sqlerrm := '';
  :sqlerrtrace := '';
exception
  when others then
    rollback;
    :sqlerrcode := SQLCODE;
    :sqlerrm := sqlerrm;
    :sqlerrtrace := DBMS_UTILITY.FORMAT_ERROR_BACKTRACE();
end;`
	apexKillSession = `
begin
  kill_session.session_id:=:sess_id;
  :ret:=kill_session.kill_session_by_session_id(:out_err_msg);
exception
  when others then
    if sqlcode = -00031 then
	  :ret := 1;
	else
      :ret := 0;
      :out_err_msg := sqlerrm;
	end if;
end;
`
	apexFileUpload = `
declare
  l_doc_size number := :doc_size; /*Для совместимости*/
  l_pt_dc_id varchar2(40) := null; /*Для совместимости*/
  l_content_type varchar2(240) := :content_type; /*Для совместимости*/
begin
  owa.init_cgi_env(:num_params, :param_name, :param_val);
/*%s %s*/
  :ret_name := apex_util.set_blob
                (
                  p_file_name=>:name
                  ,p_mime_type=>:mime_type
                  ,p_blob_content=>:lob
                  ,p_item_id=>:item_id
                  ,p_application_id=>:application_id
                  ,p_page_id=>:page_id
                  ,p_session_id=>:session_id
                  ,p_request=>:request
                );
  :sqlerrcode := 0;
  :sqlerrm := '';
  :sqlerrtrace := '';
exception
  when others then
    rollback;
    :sqlerrcode := -20000;
    :sqlerrm := 'Unable to upload file "'||:name||'" '||sqlerrm;
    :sqlerrtrace := DBMS_UTILITY.FORMAT_ERROR_BACKTRACE();
end;`
)
