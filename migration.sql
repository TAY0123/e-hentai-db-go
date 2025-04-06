ALTER TABLE torrent MODIFY COLUMN fsize BIGINT UNSIGNED;
ALTER TABLE torrent MODIFY COLUMN added DATETIME;

ALTER TABLE gallery MODIFY COLUMN title VARCHAR(512);
ALTER TABLE gallery MODIFY COLUMN title_jpn VARCHAR(512);
ALTER TABLE gallery MODIFY COLUMN uploader VARCHAR(512);

UPDATE torrent
SET fsize = fsizestr,
    added = FROM_UNIXTIME(addedstr)
WHERE addedstr NOT LIKE '____-__-__ __:__';

UPDATE torrent
SET fsizestr = NULL,
    addedstr = NULL
WHERE fsize != 0 ;

ALTER TABLE torrent
  ADD COLUMN fsize_min BIGINT UNSIGNED,
  ADD COLUMN fsize_max BIGINT UNSIGNED;

UPDATE torrent
SET 
  fsize_min = CAST(ROUND(
      (
        CAST(SUBSTRING_INDEX(fsizestr, ' ', 1) AS DECIMAL(10,6))
        - (0.5 * POWER(10, -(
            CASE 
              WHEN INSTR(SUBSTRING_INDEX(fsizestr, ' ', 1), '.') > 0 
              THEN LENGTH(SUBSTRING_INDEX(fsizestr, ' ', 1)) - LOCATE('.', SUBSTRING_INDEX(fsizestr, ' ', 1))
              ELSE 0 
            END
        )))
      )
      *
      (
        CASE SUBSTRING_INDEX(fsizestr, ' ', -1)
          WHEN 'B'   THEN 1
          WHEN 'KiB' THEN 1024
          WHEN 'MiB' THEN 1024*1024
          WHEN 'GiB' THEN 1024*1024*1024
          WHEN 'TiB' THEN 1024*1024*1024*1024
          WHEN 'KB'  THEN 1000
          WHEN 'MB'  THEN 1000*1000
          WHEN 'GB'  THEN 1000*1000*1000
          WHEN 'TB'  THEN 1000*1000*1000*1000
          ELSE 1
        END
      )
  ) AS UNSIGNED),
  
  fsize_max = CAST(ROUND(
      (
        CAST(SUBSTRING_INDEX(fsizestr, ' ', 1) AS DECIMAL(10,6))
        + (0.5 * POWER(10, -(
            CASE 
              WHEN INSTR(SUBSTRING_INDEX(fsizestr, ' ', 1), '.') > 0 
              THEN LENGTH(SUBSTRING_INDEX(fsizestr, ' ', 1)) - LOCATE('.', SUBSTRING_INDEX(fsizestr, ' ', 1))
              ELSE 0 
            END
        )))
      )
      *
      (
        CASE SUBSTRING_INDEX(fsizestr, ' ', -1)
          WHEN 'B'   THEN 1
          WHEN 'KiB' THEN 1024
          WHEN 'MiB' THEN 1024*1024
          WHEN 'GiB' THEN 1024*1024*1024
          WHEN 'TiB' THEN 1024*1024*1024*1024
          WHEN 'KB'  THEN 1000
          WHEN 'MB'  THEN 1000*1000
          WHEN 'GB'  THEN 1000*1000*1000
          WHEN 'TB'  THEN 1000*1000*1000*1000
          ELSE 1
        END
      )
  ) AS UNSIGNED)
WHERE fsizestr IS NOT NULL;